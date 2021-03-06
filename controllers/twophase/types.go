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

package twophase

import (
	"context"
	"fmt"
	"time"

	"k8s.io/client-go/util/retry"

	"github.com/chaos-mesh/chaos-mesh/api/v1alpha1"
	ctx "github.com/chaos-mesh/chaos-mesh/pkg/router/context"
	"github.com/chaos-mesh/chaos-mesh/pkg/router/endpoint"
	"github.com/chaos-mesh/chaos-mesh/pkg/utils"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
)

const emptyString = ""

// Reconciler for the twophase reconciler
type Reconciler struct {
	endpoint.Endpoint
	ctx.Context
}

// NewReconciler would create reconciler for twophase controller
func NewReconciler(e endpoint.Endpoint, ctx ctx.Context) *Reconciler {
	return &Reconciler{
		Endpoint: e,
		Context:  ctx,
	}
}

// Reconcile is twophase reconcile implement
// kubebuilder所有的crd行为都由reconcile实现
func (r *Reconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	var err error
	now := time.Now()

	r.Log.Info("Reconciling a two phase chaos", "name", req.Name, "namespace", req.Namespace)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	_chaos := r.Object()
	if err = r.Client.Get(ctx, req.NamespacedName, _chaos); err != nil {
		r.Log.Error(err, "unable to get chaos")
		return ctrl.Result{}, err
	}
	chaos := _chaos.(v1alpha1.InnerSchedulerObject)
	//在这里会获取每个chaos type的duraion，然后周期执行
	duration, err := chaos.GetDuration()
	if err != nil {
		r.Log.Error(err, "failed to get chaos duration")
		return ctrl.Result{}, err
	}

	scheduler := chaos.GetScheduler()
	if scheduler == nil {
		r.Log.Info("Scheduler should be defined currently")
		return ctrl.Result{}, fmt.Errorf("misdefined scheduler")
	}

	if duration == nil {
		zero := 0 * time.Second
		duration = &zero
	}

	status := chaos.GetStatus()
	//根据从controller拿到的status执行相应行为
	//status.Experiment.Phase
	if chaos.IsDeleted() {
		// This chaos was deleted
		r.Log.Info("Removing self")
		err = r.Recover(ctx, req, chaos)
		if err != nil {
			r.Log.Error(err, "failed to recover chaos")
			updateFailedMessage(ctx, r, chaos, err.Error())
			return ctrl.Result{Requeue: true}, err
		}

		status.Experiment.Phase = v1alpha1.ExperimentPhaseFinished
		status.FailedMessage = emptyString
	} else if chaos.IsPaused() {
		if status.Experiment.Phase == v1alpha1.ExperimentPhaseRunning {
			r.Log.Info("Pausing")
			//停止实验并recover chaos 对象
			err = r.Recover(ctx, req, chaos)
			if err != nil {
				r.Log.Error(err, "failed to pause chaos")
				updateFailedMessage(ctx, r, chaos, err.Error())
				return ctrl.Result{Requeue: true}, err
			}

			now := time.Now()
			status.Experiment.EndTime = &metav1.Time{
				Time: now,
			}
			if status.Experiment.StartTime != nil {
				status.Experiment.Duration = now.Sub(status.Experiment.StartTime.Time).String()
			}
		}
		status.Experiment.Phase = v1alpha1.ExperimentPhasePaused
		status.FailedMessage = emptyString
	} else if !chaos.GetNextRecover().IsZero() && chaos.GetNextRecover().Before(now) {
		// Start recover
		r.Log.Info("Recovering")

		// Don't need to recover again if chaos was paused before
		if status.Experiment.Phase != v1alpha1.ExperimentPhasePaused {
			if err = r.Recover(ctx, req, chaos); err != nil {
				r.Log.Error(err, "failed to recover chaos")
				updateFailedMessage(ctx, r, chaos, err.Error())
				return ctrl.Result{Requeue: true}, err
			}
		}

		chaos.SetNextRecover(time.Time{})

		status.Experiment.EndTime = &metav1.Time{
			Time: time.Now(),
		}
		status.Experiment.Phase = v1alpha1.ExperimentPhaseWaiting
		status.FailedMessage = emptyString
	} else if (status.Experiment.Phase == v1alpha1.ExperimentPhaseFailed ||
		status.Experiment.Phase == v1alpha1.ExperimentPhasePaused) &&
		!chaos.GetNextRecover().IsZero() && chaos.GetNextRecover().After(now) {
		// Only resume/retry chaos in the case when current round is not finished,
		// which means the current time is before recover time. Otherwise we
		// don't resume the chaos and just wait for the start of next round.

		r.Log.Info("Resuming/Retrying")

		dur := chaos.GetNextRecover().Sub(now)
		if err = applyAction(ctx, r, req, dur, chaos); err != nil {
			updateFailedMessage(ctx, r, chaos, err.Error())
			return ctrl.Result{Requeue: true}, err
		}

		status.FailedMessage = emptyString
	} else if chaos.GetNextStart().Before(now) {
		r.Log.Info("Starting")

		tempStart, err := utils.NextTime(*chaos.GetScheduler(), now)
		if err != nil {
			r.Log.Error(err, "failed to calculate the start time")
			updateFailedMessage(ctx, r, chaos, err.Error())
			return ctrl.Result{}, err
		}

		tempRecover := now.Add(*duration)
		if tempStart.Before(tempRecover) {
			err := fmt.Errorf("nextRecover shouldn't be later than nextStart")
			r.Log.Error(err, "Then recover can never be reached.", "scheduler", *chaos.GetScheduler(), "duration", *duration)
			updateFailedMessage(ctx, r, chaos, err.Error())
			return ctrl.Result{}, err
		}
		//applyAction将会执行我们重载的apply函数的逻辑
		if err = applyAction(ctx, r, req, *duration, chaos); err != nil {
			updateFailedMessage(ctx, r, chaos, err.Error())
			return ctrl.Result{Requeue: true}, err
		}

		nextStart, err := utils.NextTime(*chaos.GetScheduler(), status.Experiment.StartTime.Time)
		if err != nil {
			r.Log.Error(err, "failed to get the next start time")
			return ctrl.Result{}, err
		}
		nextRecover := status.Experiment.StartTime.Time.Add(*duration)

		chaos.SetNextStart(*nextStart)
		chaos.SetNextRecover(nextRecover)
		status.FailedMessage = emptyString
	} else {
		r.Log.Info("Waiting")

		nextStart, err := utils.NextTime(*chaos.GetScheduler(), status.Experiment.StartTime.Time)
		if err != nil {
			r.Log.Error(err, "failed to get next start time")
			return ctrl.Result{}, err
		}
		nextTime := chaos.GetNextStart()

		// if nextStart is not equal to nextTime, the scheduler may have been modified.
		// So set nextStart to time.Now.
		if nextStart.Equal(nextTime) {
			if !chaos.GetNextRecover().IsZero() && chaos.GetNextRecover().Before(nextTime) {
				nextTime = chaos.GetNextRecover()
			}
			duration := nextTime.Sub(now)
			r.Log.Info("Requeue request", "after", duration)

			return ctrl.Result{RequeueAfter: duration}, nil
		}

		chaos.SetNextStart(time.Now())
		duration := nextTime.Sub(now)
		r.Log.Info("Requeue request", "after", duration)
		return ctrl.Result{RequeueAfter: duration}, nil
	}

	if err := r.Update(ctx, chaos); err != nil {
		r.Log.Error(err, "unable to update chaos status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func applyAction(
	ctx context.Context,
	r *Reconciler,
	req ctrl.Request,
	duration time.Duration,
	chaos v1alpha1.InnerSchedulerObject,
) error {
	status := chaos.GetStatus()
	r.Log.Info("Chaos action:", "chaos", chaos)

	// Start to apply action
	r.Log.Info("Performing Action")
	//reconciler.Apply执行这个chaos
	//这个Apply就是我们重载的函数
	if err := r.Apply(ctx, req, chaos); err != nil {
		r.Log.Error(err, "failed to apply chaos action")

		status.Experiment.Phase = v1alpha1.ExperimentPhaseFailed

		updateError := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			return r.Update(ctx, chaos)
		})
		if updateError != nil {
			r.Log.Error(updateError, "unable to update chaos finalizers")
		}

		return err
	}

	status.Experiment.StartTime = &metav1.Time{Time: time.Now()}
	status.Experiment.Phase = v1alpha1.ExperimentPhaseRunning
	status.Experiment.Duration = duration.String()
	return nil
}

func updateFailedMessage(
	ctx context.Context,
	r *Reconciler,
	chaos v1alpha1.InnerSchedulerObject,
	err string,
) {
	status := chaos.GetStatus()
	status.FailedMessage = err
	if err := r.Update(ctx, chaos); err != nil {
		r.Log.Error(err, "unable to update chaos status")
	}
}
