package golangchaos

import (
	"context"
	"errors"
	"fmt"
	"time"

	"golang.org/x/sync/errgroup"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/chaos-mesh/chaos-mesh/api/v1alpha1"
	"github.com/chaos-mesh/chaos-mesh/controllers/common"
	pb "github.com/chaos-mesh/chaos-mesh/pkg/chaosdaemon/pb"
	"github.com/chaos-mesh/chaos-mesh/pkg/router"
	ctx "github.com/chaos-mesh/chaos-mesh/pkg/router/context"
	end "github.com/chaos-mesh/chaos-mesh/pkg/router/endpoint"
	"github.com/chaos-mesh/chaos-mesh/pkg/utils"
)

// Copyright 2020 Chaos Mesh Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// See the License for the specific language governing permissions and
// limitations under the License.

const (
	golangSqlQueryErrorActionMsg = "set sql query error to %s"
)

type endpoint struct {
	ctx.Context
}

// Apply implements the reconciler.InnerReconciler.Apply
func (r *endpoint) Apply(ctx context.Context, req ctrl.Request, obj v1alpha1.InnerObject) error {
	var err error

	golangChaos, ok := obj.(*v1alpha1.GolangChaos)
	if !ok {
		err = errors.New("chaos is not golang chaos")
		r.Log.Error(err, "chaos is not golang chaos", "chaos", obj)
		return err
	}
	allContainers := false
	containerMap := make(map[string]bool)
	//如果containers为空，就需要将所有的pod中的container注入异常，除了pause
	if len(golangChaos.Spec.ContainerNames) == 0 {
		r.Log.Info("golangchaos.ContainerNames is empty , ready to set all containers.")
		allContainers = true
	} else {
		for i := range golangChaos.Spec.ContainerNames {
			containerMap[golangChaos.Spec.ContainerNames[i]] = true
		}
	}

	pods, err := utils.SelectAndFilterPods(ctx, r.Client, r.Reader, &golangChaos.Spec)
	if err != nil {
		r.Log.Error(err, "fail to select and filter pods")
		return err
	}

	g := errgroup.Group{}
	for podIndex := range pods {
		pod := &pods[podIndex]

		for _, container := range pod.Status.ContainerStatuses {
			containerName := container.Name
			containerID := container.ContainerID
			_, ok = containerMap[containerName]
			if (allContainers && containerName != "pause") || ok {

				g.Go(func() error {
					switch golangChaos.Spec.Action {
					case v1alpha1.SqlErrorAction:
						duration, err := golangChaos.GetDuration()
						if err != nil {
							r.Log.Error(err, fmt.Sprintf(
								"failed to get duration: %s, pod: %s, namespace: %s",
								containerName, pod.Name, pod.Namespace))
						}
						err = r.GolangSetSqlError(ctx, pod, containerID, *duration)
						if err != nil {
							r.Log.Error(err, fmt.Sprintf(
								"failed to set golang to container :%s ,  error : %s, pod: %s, namespace: %s",
								containerName, pod.Name, pod.Namespace, containerName))
						}
						//设置golangChaos失败并不影响继续运行，有可能设置到其他非golang容器上了
						return nil
					default:
						err := errors.New("Unknow golang chaos action")
						r.Log.Error(err, fmt.Sprintf("unknow golang chaos action"))
						return err
					}
				})
			}
		}
	}

	if err = g.Wait(); err != nil {
		return err
	}

	golangChaos.Status.Experiment.PodRecords = make([]v1alpha1.PodStatus, 0, len(pods))
	msg := ""
	for i := range golangChaos.Spec.ContainerNames {
		msg += " " + golangChaos.Spec.ContainerNames[i]
	}
	for _, pod := range pods {
		ps := v1alpha1.PodStatus{
			Namespace: pod.Namespace,
			Name:      pod.Name,
			HostIP:    pod.Status.HostIP,
			PodIP:     pod.Status.PodIP,
			Action:    string(golangChaos.Spec.Action),
			Message:   fmt.Sprintf(golangSqlQueryErrorActionMsg, msg),
		}

		golangChaos.Status.Experiment.PodRecords = append(golangChaos.Status.Experiment.PodRecords, ps)
	}
	r.Event(obj, v1.EventTypeNormal, utils.EventChaosInjected, "")
	return nil
}

//设置golang sql driver异常
func (r *endpoint) GolangSetSqlError(ctx context.Context, pod *v1.Pod, containerID string, duration time.Duration) error {
	r.Log.Info("Trying to set golang error", "namespace", pod.Namespace, "podName", pod.Name, "containerID", containerID)

	pbClient, err := utils.NewChaosDaemonClient(ctx, r.Client, pod, common.ControllerCfg.ChaosDaemonPort)

	if err != nil {
		return err
	}
	defer pbClient.Close()

	if len(pod.Status.ContainerStatuses) == 0 {
		return fmt.Errorf("%s %s can't get the state of container", pod.Namespace, pod.Name)
	}
	//将其转化成秒
	seconds := int64(duration.Seconds())
	response, err := pbClient.SetGolangError(ctx, &pb.GolangErrorRequest{Action: &pb.GolangErrorAction{Action: pb.GolangErrorAction_SqlErrorAction}, ContainerId: containerID, Duration: seconds})

	if err != nil {
		r.Log.Error(err, "set golang exception error", "namespace", pod.Namespace, "podName", pod.Name, "containerID", containerID)
		return err
	}

	r.Log.Info("Set golang exception to process : ", "pid", response.Pid)

	return nil
}

func init() {
	router.Register("golangchaos", &v1alpha1.GolangChaos{}, func(obj runtime.Object) bool {
		_, ok := obj.(*v1alpha1.GolangChaos)
		if !ok {
			return false
		}

		return true
	}, func(ctx ctx.Context) end.Endpoint {
		return &endpoint{
			Context: ctx,
		}
	})
}

// Recover implements the reconciler.InnerReconciler.Recover
//恢复pod/container状态，清除delve_tool这个process
//其实没必要，到点process就被kill掉了
//如果随意清除反而会留下断点之类，确保正常退出很重要
func (r *endpoint) Recover(ctx context.Context, req ctrl.Request, obj v1alpha1.InnerObject) error {
	//golangChaos , ok := obj.(*v1alpha1.GolangChaos)
	//if !ok{
	//	err := errors.New("Failed to convert object to golang chaos")
	//	r.Log.Error(err, "Failed to convert object to golang chaos")
	//	return err
	//}
	//
	//if err := r.killDelveTool(ctx , golangChaos) ; err != nil{
	//	r.Log.Error(err , "Failed to kill delve tool process to stop experiment")
	//	return err
	//}
	//
	//r.Event(golangChaos, v1.EventTypeNormal, utils.EventChaosRecovered, "")
	return nil

}

// Object implements the reconciler.InnerReconciler.Object
func (r *endpoint) Object() v1alpha1.InnerObject {
	return &v1alpha1.GolangChaos{}
}

////清除掉daemon启动的delve tool
//func (r *endpoint) killDelveTool(ctx context.Context, golangChaos *v1alpha1.GolangChaos)  error {
//
//}
