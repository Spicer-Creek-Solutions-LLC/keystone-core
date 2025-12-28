package k8s

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=rexec
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Pods",type=integer,JSONPath=`.status.podsExecuted`
// +kubebuilder:printcolumn:name="Succeeded",type=integer,JSONPath=`.status.podsSucceeded`
// +kubebuilder:printcolumn:name="Failed",type=integer,JSONPath=`.status.podsFailed`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// RemoteExecution is the Schema for the remoteexecutions API
type RemoteExecution struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RemoteExecutionSpec   `json:"spec,omitempty"`
	Status RemoteExecutionStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// RemoteExecutionList contains a list of RemoteExecution
type RemoteExecutionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RemoteExecution `json:"items"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=sconfig
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Pods",type=integer,JSONPath=`.status.podsApplied`
// +kubebuilder:printcolumn:name="Drift",type=boolean,JSONPath=`.status.driftDetected`
// +kubebuilder:printcolumn:name="Last Applied",type=date,JSONPath=`.status.lastApplied`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// StateConfig is the Schema for the stateconfigs API
type StateConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   StateConfigSpec   `json:"spec,omitempty"`
	Status StateConfigStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// StateConfigList contains a list of StateConfig
type StateConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []StateConfig `json:"items"`
}

// CRD manifests for Kubernetes deployment

const RemoteExecutionCRD = `
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: remoteexecutions.kscore.io
spec:
  group: kscore.io
  names:
    kind: RemoteExecution
    listKind: RemoteExecutionList
    plural: remoteexecutions
    singular: remoteexecution
    shortNames:
    - rexec
  scope: Namespaced
  versions:
  - name: v1
    served: true
    storage: true
    schema:
      openAPIV3Schema:
        type: object
        properties:
          spec:
            type: object
            required:
            - target
            - command
            properties:
              target:
                type: object
                properties:
                  namespace:
                    type: string
                  labelSelector:
                    type: string
                  fieldSelector:
                    type: string
                  names:
                    type: array
                    items:
                      type: string
                  container:
                    type: string
                  maxPods:
                    type: integer
              command:
                type: array
                items:
                  type: string
              schedule:
                type: string
                pattern: '^(\*|([0-9]|1[0-9]|2[0-9]|3[0-9]|4[0-9]|5[0-9])|\*\/([0-9]|1[0-9]|2[0-9]|3[0-9]|4[0-9]|5[0-9])) (\*|([0-9]|1[0-9]|2[0-3])|\*\/([0-9]|1[0-9]|2[0-3])) (\*|([1-9]|1[0-9]|2[0-9]|3[0-1])|\*\/([1-9]|1[0-9]|2[0-9]|3[0-1])) (\*|([1-9]|1[0-2])|\*\/([1-9]|1[0-2])) (\*|([0-6])|\*\/([0-6]))$'
              timeout:
                type: string
              mode:
                type: string
                enum:
                - pod
                - job
                - node
          status:
            type: object
            properties:
              phase:
                type: string
              startTime:
                type: string
                format: date-time
              completionTime:
                type: string
                format: date-time
              podsExecuted:
                type: integer
              podsSucceeded:
                type: integer
              podsFailed:
                type: integer
              message:
                type: string
              results:
                type: array
                items:
                  type: object
                  properties:
                    podName:
                      type: string
                    namespace:
                      type: string
                    exitCode:
                      type: integer
                    output:
                      type: string
                    error:
                      type: string
                    duration:
                      type: string
    additionalPrinterColumns:
    - name: Phase
      type: string
      jsonPath: .status.phase
    - name: Pods
      type: integer
      jsonPath: .status.podsExecuted
    - name: Succeeded
      type: integer
      jsonPath: .status.podsSucceeded
    - name: Failed
      type: integer
      jsonPath: .status.podsFailed
    - name: Age
      type: date
      jsonPath: .metadata.creationTimestamp
    subresources:
      status: {}
`

const StateConfigCRD = `
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: stateconfigs.kscore.io
spec:
  group: kscore.io
  names:
    kind: StateConfig
    listKind: StateConfigList
    plural: stateconfigs
    singular: stateconfig
    shortNames:
    - sconfig
  scope: Namespaced
  versions:
  - name: v1
    served: true
    storage: true
    schema:
      openAPIV3Schema:
        type: object
        properties:
          spec:
            type: object
            required:
            - target
            - states
            properties:
              target:
                type: object
                properties:
                  namespace:
                    type: string
                  labelSelector:
                    type: string
                  fieldSelector:
                    type: string
                  names:
                    type: array
                    items:
                      type: string
                  container:
                    type: string
                  maxPods:
                    type: integer
              states:
                type: array
                items:
                  type: object
                  required:
                  - name
                  - module
                  properties:
                    name:
                      type: string
                    module:
                      type: string
                    parameters:
                      type: object
                      x-kubernetes-preserve-unknown-fields: true
                    requisites:
                      type: object
                      additionalProperties:
                        type: array
                        items:
                          type: string
              vars:
                type: object
                x-kubernetes-preserve-unknown-fields: true
              schedule:
                type: string
                pattern: '^(\*|([0-9]|1[0-9]|2[0-9]|3[0-9]|4[0-9]|5[0-9])|\*\/([0-9]|1[0-9]|2[0-9]|3[0-9]|4[0-9]|5[0-9])) (\*|([0-9]|1[0-9]|2[0-3])|\*\/([0-9]|1[0-9]|2[0-3])) (\*|([1-9]|1[0-9]|2[0-9]|3[0-1])|\*\/([1-9]|1[0-9]|2[0-9]|3[0-1])) (\*|([1-9]|1[0-2])|\*\/([1-9]|1[0-2])) (\*|([0-6])|\*\/([0-6]))$'
          status:
            type: object
            properties:
              phase:
                type: string
              lastApplied:
                type: string
                format: date-time
              podsApplied:
                type: integer
              podsSucceeded:
                type: integer
              podsFailed:
                type: integer
              message:
                type: string
              driftDetected:
                type: boolean
    additionalPrinterColumns:
    - name: Phase
      type: string
      jsonPath: .status.phase
    - name: Pods
      type: integer
      jsonPath: .status.podsApplied
    - name: Drift
      type: boolean
      jsonPath: .status.driftDetected
    - name: Last Applied
      type: date
      jsonPath: .status.lastApplied
    - name: Age
      type: date
      jsonPath: .metadata.creationTimestamp
    subresources:
      status: {}
`
