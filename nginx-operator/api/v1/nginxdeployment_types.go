package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NginxDeploymentSpec defines the desired state of NginxDeployment
type NginxDeploymentSpec struct {
	// Number of nginx replicas
	Replicas int32 `json:"replicas"`

	// Port for nginx container
	Port int32 `json:"port,omitempty"`

	// Docker image for nginx
	Image string `json:"image,omitempty"`
}

// NginxDeploymentStatus defines the observed state of NginxDeployment
type NginxDeploymentStatus struct {
	// Number of available replicas
	AvailableReplicas int32 `json:"availableReplicas"`

	// Status message
	Status string `json:"status,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// NginxDeployment is the Schema for the nginxdeployments API
type NginxDeployment struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NginxDeploymentSpec   `json:"spec,omitempty"`
	Status NginxDeploymentStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// NginxDeploymentList contains a list of NginxDeployment
type NginxDeploymentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NginxDeployment `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NginxDeployment{}, &NginxDeploymentList{})
}
