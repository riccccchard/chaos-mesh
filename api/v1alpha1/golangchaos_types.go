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
	SqlErrorAction GolangChaosAction = "sql-query-error"
)

// GolangChaosSpec defines the attributes that a user creates on a chaos experiment about golang inject.
type GolangChaosSpec struct {
	// Selector is used to select pods that are used to inject chaos action.
	Selector SelectorSpec `json:"selector"`

	// Scheduler defines some schedule rules to
	// control the running time of the chaos experiment about pods.
	Scheduler *SchedulerSpec `json:"scheduler,omitempty"`

	// Action defines the specific pod chaos action.
	// Supported action: sql-query-error
	// Default action: sql-query-error
	// +kubebuilder:validation:Enum=sql-query-error
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
