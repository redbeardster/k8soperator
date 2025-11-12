package controller

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	webv1 "github.com/redbeardster/nginx-operator/api/v1"
)

// NginxDeploymentReconciler reconciles a NginxDeployment object
type NginxDeploymentReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=web.example.com,resources=nginxdeployments,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=web.example.com,resources=nginxdeployments/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=web.example.com,resources=nginxdeployments/finalizers,verbs=update
//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete

func (r *NginxDeploymentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// Get the NginxDeployment instance
	var nginxDeploy webv1.NginxDeployment
	if err := r.Get(ctx, req.NamespacedName, &nginxDeploy); err != nil {
		if errors.IsNotFound(err) {
			log.Info("NginxDeployment resource not found - deleted")
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to get NginxDeployment")
		return ctrl.Result{}, err
	}

	// Set defaults
	if nginxDeploy.Spec.Image == "" {
		nginxDeploy.Spec.Image = "nginx:latest"
	}
	if nginxDeploy.Spec.Port == 0 {
		nginxDeploy.Spec.Port = 80
	}

	// Reconcile Deployment
	if err := r.reconcileDeployment(ctx, &nginxDeploy); err != nil {
		log.Error(err, "Failed to reconcile Deployment")
		return ctrl.Result{}, err
	}

	// Reconcile Service
	if err := r.reconcileService(ctx, &nginxDeploy); err != nil {
		log.Error(err, "Failed to reconcile Service")
		return ctrl.Result{}, err
	}

	// Update status
	if err := r.updateStatus(ctx, &nginxDeploy); err != nil {
		log.Error(err, "Failed to update status")
		return ctrl.Result{}, err
	}

	log.Info("Successfully reconciled NginxDeployment")
	return ctrl.Result{}, nil
}

func (r *NginxDeploymentReconciler) reconcileDeployment(ctx context.Context, nginxDeploy *webv1.NginxDeployment) error {
	log := log.FromContext(ctx)

	targetPort := intstr.FromInt(int(nginxDeploy.Spec.Port))

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      nginxDeploy.Name + "-deployment",
			Namespace: nginxDeploy.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &nginxDeploy.Spec.Replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": nginxDeploy.Name},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": nginxDeploy.Name},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "nginx",
							Image: nginxDeploy.Spec.Image,
							Ports: []corev1.ContainerPort{
								{
									ContainerPort: nginxDeploy.Spec.Port,
								},
							},
							LivenessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/",
										Port: targetPort,
									},
								},
								InitialDelaySeconds: 15,
								TimeoutSeconds:      5,
							},
							ReadinessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/",
										Port: targetPort,
									},
								},
								InitialDelaySeconds: 5,
								TimeoutSeconds:      5,
							},
						},
					},
				},
			},
		},
	}

	// Set controller reference
	if err := ctrl.SetControllerReference(nginxDeploy, deployment, r.Scheme); err != nil {
		return err
	}

	// Check if deployment exists
	foundDeploy := &appsv1.Deployment{}
	err := r.Get(ctx, types.NamespacedName{
		Name:      deployment.Name,
		Namespace: deployment.Namespace,
	}, foundDeploy)

	if err != nil && errors.IsNotFound(err) {
		log.Info("Creating Deployment", "name", deployment.Name)
		return r.Create(ctx, deployment)
	} else if err != nil {
		return err
	}

	// Update if needed
	if *foundDeploy.Spec.Replicas != *deployment.Spec.Replicas ||
		foundDeploy.Spec.Template.Spec.Containers[0].Image != deployment.Spec.Template.Spec.Containers[0].Image {

		log.Info("Updating Deployment", "name", deployment.Name)
		foundDeploy.Spec = deployment.Spec
		return r.Update(ctx, foundDeploy)
	}

	return nil
}

func (r *NginxDeploymentReconciler) reconcileService(ctx context.Context, nginxDeploy *webv1.NginxDeployment) error {
	log := log.FromContext(ctx)

	targetPort := intstr.FromInt(int(nginxDeploy.Spec.Port))

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      nginxDeploy.Name + "-service",
			Namespace: nginxDeploy.Namespace,
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{"app": nginxDeploy.Name},
			Ports: []corev1.ServicePort{
				{
					Port:       nginxDeploy.Spec.Port,
					TargetPort: targetPort,
				},
			},
			Type: corev1.ServiceTypeClusterIP,
		},
	}

	// Set controller reference
	if err := ctrl.SetControllerReference(nginxDeploy, service, r.Scheme); err != nil {
		return err
	}

	foundService := &corev1.Service{}
	err := r.Get(ctx, types.NamespacedName{
		Name:      service.Name,
		Namespace: service.Namespace,
	}, foundService)

	if err != nil && errors.IsNotFound(err) {
		log.Info("Creating Service", "name", service.Name)
		return r.Create(ctx, service)
	} else if err != nil {
		return err
	}

	return nil
}

func (r *NginxDeploymentReconciler) updateStatus(ctx context.Context, nginxDeploy *webv1.NginxDeployment) error {
	deployment := &appsv1.Deployment{}
	err := r.Get(ctx, types.NamespacedName{
		Name:      nginxDeploy.Name + "-deployment",
		Namespace: nginxDeploy.Namespace,
	}, deployment)

	if err != nil {
		return err
	}

	nginxDeploy.Status.AvailableReplicas = deployment.Status.AvailableReplicas

	if deployment.Status.AvailableReplicas == nginxDeploy.Spec.Replicas {
		nginxDeploy.Status.Status = "Ready"
	} else {
		nginxDeploy.Status.Status = fmt.Sprintf("Available: %d/%d",
			deployment.Status.AvailableReplicas, nginxDeploy.Spec.Replicas)
	}

	return r.Status().Update(ctx, nginxDeploy)
}

func (r *NginxDeploymentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&webv1.NginxDeployment{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Complete(r)
}
