#  添加GolangChaos异常类型

##  golang chaos工作流程

Golang chaos 模仿chaos mesh的其他异常类型的注入方式，意在复用chaos mesh实现的接口和功能，在chaos mesh上做拓展。

通过实现golang chaos类型的crd，通过k8s的yaml定义实验参数并注入，即可生成实验。

大致流程为：

1. kube-apiserver接受用户定义的yaml文件，转化为对应GolangChaos对象，在controller manager中调用Golang Chaos对象实现的Apply函数完成调度；

2. GolangChaos.Apply函数中将与chaos daemon通信，在chaos daemon中启动二进制工具delve_tool完成实验注入。

##  如何添加golang异常类型

接下来，简单介绍一下golangchaos如何添加。

###   1.  定义crd type

首先在chaos-mesh/api/v1alpha1目录下实现golangchaos_types.go

```go
// Copyright 2019 Chaos Mesh Authors.
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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:object:root=true
// +chaos-mesh:base

// Golangchaos is the control script`s spec.
type GolangChaos struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec defines the behavior of a golang chaos experiment
	Spec GolangChaosSpec `json:"spec"`

	// +optional
	// Most recently observed status of the chaos experiment about golang
	Status GolangChaosStatus `json:"status"`
}

// GolangChaosAction represents the chaos action about golang injects.
type GolangChaosAction string

const (
	//sql query error 表示将sql.(*DB).Query的error返回值设置为非nil
	SqlErrorAction GolangChaosAction = "sql-error"
)

// GolangChaosSpec defines the attributes that a user creates on a chaos experiment about golang inject.
type GolangChaosSpec struct {
	// Selector is used to select pods that are used to inject chaos action.
	Selector SelectorSpec `json:"selector"`

	// Scheduler defines some schedule rules to
	// control the running time of the chaos experiment about pods.
	Scheduler *SchedulerSpec `json:"scheduler,omitempty"`

	// Action defines the specific pod chaos action.
	// Supported action: sql-error
	// Default action: sql-error
	// +kubebuilder:validation:Enum=sql-error
	Action GolangChaosAction `json:"action"`

	// Mode defines the mode to run chaos action.
	// Supported mode: one / all / fixed / fixed-percent / random-max-percent
	// +kubebuilder:validation:Enum=one;all;fixed;fixed-percent;random-max-percent
	Mode PodMode `json:"mode"`

	// Value is required when the mode is set to `FixedPodMode` / `FixedPercentPodMod` / `RandomMaxPercentPodMod`.
	// If `FixedPodMode`, provide an integer of pods to do chaos action.
	// If `FixedPercentPodMod`, provide a number from 0-100 to specify the percent of pods the server can do chaos action.
	// IF `RandomMaxPercentPodMod`,  provide a number from 0-100 to specify the max percent of pods to do chaos action
	// +optional
	Value string `json:"value"`

	// Duration represents the duration of the chaos action.
	// It is required when the action is `SqlErrorAction`.
	// A duration string is a possibly signed sequence of
	// decimal numbers, each with optional fraction and a unit suffix,
	// such as "300ms", "-1.5h" or "2h45m".
	// Valid time units are "ns", "us" (or "µs"), "ms", "s", "m", "h".
	// +optional
	Duration *string `json:"duration,omitempty"`

	// ContainerNames表示需要hack的多个containers,
	// 如果不指明，chaos mesh将会将pod中所有的containers(除了pause)注入异常
	// +optional
	ContainerNames []string `json:"containerName,omitempty"`

	// GracePeriod is used in pod-kill action. It represents the duration in seconds before the pod should be deleted.
	// Value must be non-negative integer. The default value is zero that indicates delete immediately.
	// +optional
	// +kubebuilder:validation:Minimum=0
	GracePeriod int64 `json:"gracePeriod"`
}

func (in *GolangChaosSpec) GetSelector() SelectorSpec {
	return in.Selector
}

func (in *GolangChaosSpec) GetMode() PodMode {
	return in.Mode
}

func (in *GolangChaosSpec) GetValue() string {
	return in.Value
}

// GolangChaosStatus represents the current status of the chaos experiment about pods.
type GolangChaosStatus struct {
	ChaosStatus `json:",inline"`
}

//// PodStatus represents information about the status of a pod in chaos experiment.
//type PodStatus struct {
//	Namespace string `json:"namespace"`
//	Name      string `json:"name"`
//	Action    string `json:"action"`
//	HostIP    string `json:"hostIP"`
//	PodIP     string `json:"podIP"`
//
//	// A brief CamelCase message indicating details about the chaos action.
//	// e.g. "delete this pod" or "pause this pod duration 5m"
//	// +optional
//	Message string `json:"message"`
//}

```

内容主要为GolangChaos的参数定义和重载函数；

**selector**寻找对应pod；

**containerName**为需要attach的container名；

**duration为**实验持续时间；

**Action**表示实验类型，定义在GolangChaosAction常数中，如"sql-query-error"

### 2. 通过chaos-mesh官方工具生成对应的yaml模板文件

在chaos-mesh目录下执行：

```bash
make yaml
```

将在chaos-mesh/config/crd/bases下生成chaos-mesh.org_golangchaos.yaml模板文件，各种参数都有说明。

随后，将**chaos-mesh.org_golangchaos.yaml**的内容加入到**chaos-mesh/manifests/crd.yaml**中。

### 3. 通过chaos-mesh官方工具，生成重载函数

在chaos-mesh目录下执行

```bash
make generate
```

将在**chaos-mesh/api/v1alpha1/zz_generated.chaosmesh.go**和

**chaos-mesh/api/v1alpha1/zz_generated.deepcopy.go**中

生成一系列重载函数。

### 4.  在**chaos-mesh/controllers**目录下添加golangchaos-controller的逻辑

新建文件夹golangchaos，在其下新建文件types.go，填入以下内容

```go
package golangchaos

import (
	"context"
	"errors"
	"fmt"
	"golang.org/x/sync/errgroup"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"time"

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
func (r *endpoint) Apply(ctx context.Context , req ctrl.Request , obj v1alpha1.InnerObject) error{
	var err error

	golangChaos , ok := obj.(*v1alpha1.GolangChaos)
	if !ok{
		err = errors.New("chaos is not golang chaos")
		r.Log.Error(err, "chaos is not golang chaos", "chaos", obj)
		return err
	}
	allContainers := false
	containerMap := make(map[string]bool)
	//如果containers为空，就需要将所有的pod中的container注入异常，除了pause
	if len( golangChaos.Spec.ContainerNames) == 0{
		r.Log.Info("golangchaos.ContainerNames is empty , ready to set all containers.")
		allContainers = true
	}else{
		for i := range golangChaos.Spec.ContainerNames{
			containerMap[golangChaos.Spec.ContainerNames[i]] = true
		}
	}

	pods , err := utils.SelectAndFilterPods(ctx, r.Client, r.Reader , &golangChaos.Spec)
	if err != nil{
		r.Log.Error(err, "fail to select and filter pods")
		return err
	}

	g := errgroup.Group{}
	for podIndex := range pods{
		pod := &pods[podIndex]

		for _ , container := range pod.Status.ContainerStatuses{
			containerName := container.Name
			containerID := container.ContainerID
			_ , ok = containerMap[containerName]
			if (allContainers && containerName != "pause") || ok {

				g.Go( func( ) error{
					switch golangChaos.Spec.Action{
					case v1alpha1.SqlErrorAction:
						duration , err := golangChaos.GetDuration()
						if err != nil{
							r.Log.Error(err, fmt.Sprintf(
								"failed to get duration: %s, pod: %s, namespace: %s",
								containerName, pod.Name, pod.Namespace))
						}
						err = r.GolangSetSqlError(ctx, pod , containerID , *duration)
						if err != nil {
							r.Log.Error(err, fmt.Sprintf(
								"failed to set golang to container :%s ,  error : %s, pod: %s, namespace: %s",
								containerName, pod.Name, pod.Namespace , containerName))
						}
						//设置golangChaos失败并不影响继续运行，有可能设置到其他非golang容器上了
						return nil
					default:
						err := errors.New("Unknow golang chaos action")
						r.Log.Error(err , fmt.Sprintf("unknow golang chaos action"))
						return err
					}
				})
			}
		}
	}

	if err = g.Wait() ; err != nil{
		return err
	}

	golangChaos.Status.Experiment.PodRecords = make([]v1alpha1.PodStatus, 0, len(pods))
	msg := ""
	for i := range golangChaos.Spec.ContainerNames{
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
func (r *endpoint) GolangSetSqlError(ctx context.Context , pod *v1.Pod , containerID string, duration time.Duration) error{
	r.Log.Info("Trying to set golang error", "namespace", pod.Namespace, "podName", pod.Name, "containerID", containerID )

	pbClient , err := utils.NewChaosDaemonClient(ctx, r.Client, pod , common.ControllerCfg.ChaosDaemonPort)

	if err != nil{
		return err
	}
	defer pbClient.Close()

	if len(pod.Status.ContainerStatuses) == 0 {
		return fmt.Errorf("%s %s can't get the state of container", pod.Namespace, pod.Name)
	}
	//将其转化成秒
	seconds := int64(duration.Seconds())
	response , err := pbClient.SetGolangError(ctx, &pb.GolangErrorRequest{Action: &pb.GolangErrorAction{Action: pb.GolangErrorAction_SqlErrorAction} , ContainerId: containerID, Duration: seconds})

	if err != nil{
		r.Log.Error(err, "set golang exception error", "namespace", pod.Namespace, "podName", pod.Name, "containerID", containerID)
		return err
	}

	r.Log.Info("Set golang exception to process : ", "pid", response.Pid)

	return nil
}

func init() {
	router.Register("golangchaos", &v1alpha1.GolangChaos{}, func(obj runtime.Object) bool {
		_ , ok := obj.(*v1alpha1.GolangChaos)
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

```

此代码主要重载了reconciler.InnerReconciler的三个函数，其中最重要的为apply函数，其将在controller调度我们的crd对象的时候执行。

因此，在apply函数中，我们获取golang异常实验的参数，根据参数执行相应的实验：通过pbclient与Chaosdaemon通信。

### 5.  我们在**chaos-mesh/cmd/controller-manager/main.go**中引入我们的golangchaos-controller



```go
import _ "github.com/chaos-mesh/chaos-mesh/controllers/golangchaos"
```

**至此，controller-manager的逻辑基本实现完成。**

**接下来我们实现chaosdaemon的逻辑。**

### 6.  在chaos daemon中拓展功能

chaos daemon本质上为一个grpc server，所以很方便拓展。

在chaos-mesh/pkg/chaosdaemon下可以看见pb文件，在chaosdaemon.proto中添加我们的功能

```protobuf
rpc SetGolangError (GolangErrorRequest) returns (GolangErrorResponse) {}

message GolangErrorRequest{
  GolangErrorAction action = 1;
  string container_id = 2;
  //持续时间，换算成秒
  int64 duration = 3;
}
message GolangErrorResponse{
  //返回注入给了哪个进程
  uint32 pid = 1;
  //开始的时间
  int64 startTime = 2;
}

message GolangErrorAction{
  enum Action{
      SqlErrorAction = 0;
  }
  Action action= 1;
}
```

然后，我们新建golang_error.go函数完成此功能

```go
package chaosdaemon

import (
	"context"
	"fmt"
	"github.com/chaos-mesh/chaos-mesh/pkg/bpm"
	"github.com/shirou/gopsutil/process"
	"os"
	"strings"

	pb "github.com/chaos-mesh/chaos-mesh/pkg/chaosdaemon/pb"
)

const(
	delve_tool_Bin = "/usr/local/bin/delve_tool"
)

func getErrorType(action pb.GolangErrorAction_Action)int {
	switch action {
	case pb.GolangErrorAction_SqlErrorAction:
		return 0
	}
	return -1
}
//设置golang异常
//0 : 表示sql error action，将会调用delve_tool修改dababase/sql.(*DB)的所有函数的返回值
func (s *daemonServer) SetGolangError(ctx context.Context, request *pb.GolangErrorRequest) (*pb.GolangErrorResponse, error) {
	log.Info("trying to set golang error to target container , ", "containerID", request.ContainerId , "Action" , request.Action)

	//错误类型
	errorType := getErrorType(request.Action.Action)
	//获取需要attach 的pid
	pid, err := s.crClient.GetPidFromContainerID(ctx, request.ContainerId)
	if err != nil {
		log.Error(err, "error while getting PID")
		return nil, err
	}
	//转化成秒
	duration := fmt.Sprintf("%ds", request.Duration)
	//delve server端口监听地址
	address := "127.0.0.1:30303"

	//运行delve tool 的参数
	args := fmt.Sprintf("--pid %v --address %s --type %d --duration %s", pid , address , errorType, duration)

	log.Info("executing" , "cmd" , delve_tool_Bin + " " + args)

	cmd := bpm.DefaultProcessBuilder(delve_tool_Bin , strings.Split(args , " ")...).
		EnableSuicide().
		SetIdentifier(request.ContainerId).
		Build()

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err = s.backgroundProcessManager.StartProcess(cmd)
	if err != nil {
		return nil, err
	}

	procState, err := process.NewProcess(int32(cmd.Process.Pid))
	if err != nil {
		return nil, err
	}
	ct, err := procState.CreateTime()
	if err != nil {
		if kerr := cmd.Process.Kill(); kerr != nil {
			log.Error(kerr, "kill delve tool failed", "request", request)
		}
		return nil, err
	}

	return &pb.GolangErrorResponse{
		StartTime: ct,
		Pid: pid,
	}, nil
}
```

代码的逻辑很简单，获取参数后，启动我们的二进制工具去attach目标进程即可。

### 7.  现在，我们使用如下命令添加crd内容

```bash
kubectl apply -f manifests/
kubectl get crd golangchaos.chaos-mesh.org
```

这样即可在k8s集群中添加我们的扩展资源。

###  8.  修改chaos daemon的dockerfile

我们需要在chaos daemon启动的时候将delve_tool打入进去，所以将

**chaos-mesh/images/chaos-daemon/Dockerfile**

改为

```dockerfile
FROM debian:buster-slim

ARG HTTPS_PROXY
ARG HTTP_PROXY

ENV http_proxy $HTTP_PROXY
ENV https_proxy $HTTPS_PROXY
#添加一个ps命令
RUN apt-get update && apt-get install -y tzdata procps iptables ipset stress-ng iproute2 fuse util-linux && rm -rf /var/lib/apt/lists/*

RUN update-alternatives --set iptables /usr/sbin/iptables-legacy

ADD https://github.com/riccccchard/delve_tool/releases/download/delve_tool-0.4.2/delve_tool /usr/local/bin/
#防止权限不足
RUN chmod 777 /usr/local/bin/delve_tool
ENV RUST_BACKTRACE 1

COPY --from=pingcap/chaos-binary /bin/chaos-daemon /usr/local/bin/chaos-daemon
COPY --from=pingcap/chaos-binary /bin/toda /usr/local/bin/toda
COPY --from=pingcap/chaos-binary /bin/pause /usr/local/bin/pause
COPY --from=pingcap/chaos-binary /bin/suicide /usr/local/bin/suicide
```



### 9.  现在，在chaos-mesh目录下执行

```bash
make
```

则会将我们修改的内容新建为本地镜像，名字为

```go
localhost:5000/pingcap/chaos-fs
localhost:5000/pingcap/chaos-mesh
localhost:5000/pingcap/chaos-dashboard
localhost:5000/pingcap/chaos-daemon
```

如果有需要，可以用以下命令上传镜像（非必须）

```bash
make docker_push
```

我们还可以通过修改Makefile文件来修改docker镜像的名字，
```Makefile
# Set DEBUGGER=1 to build debug symbols
LDFLAGS = $(if $(IMG_LDFLAGS),$(IMG_LDFLAGS),$(if $(DEBUGGER),,-s -w) $(shell ./hack/version.sh))
DOCKER_REGISTRY ?= "riccccchard"
```
如上，修改makefile中的DOCKER_REGISTRY为自定义名字riccccchard，就会获得以riccccchard为前缀的镜像。

### 10.  最后，修改install.sh的内容

在install.sh中文本搜索

```
pingcap
```

将**所有**的image替换为自己的image，如

```go
pingcap/chaos-daemon   ->   localhost:5000/pingcap/chaos-daemon
```

### 11.  最后，执行./install.sh即可安装上

###  12.  验证

可以使用如下yaml文件验证

```yaml
apiVersion: chaos-mesh.org/v1alpha1
kind: GolangChaos
metadata:
    name: golang-error-example
    namespace: chaos-testing
spec:
    #action为枚举类型，选择可以参考config/bases/chaos-mesh.org_golangchaos.yaml
    action: sql-error
    mode: one
    duration: "20s"
    containerNames:
        - httpapp
    selector:
        labelSelectors:
            app: httpapp
    scheduler:
        cron: "@every 2m"

```

前提是需要在集群中安装httpapp这个应用

此应用启动了一个http请求，每次curl都会使用sql库去查询数据库中的数据

代码和deploy文件放在chaos-mesh/test/httpapp文件夹中









