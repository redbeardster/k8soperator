package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"
)

type PodHealer struct {
	clientset *kubernetes.Clientset
}

func NewPodHealer() (*PodHealer, error) {
	var config *rest.Config
	var err error

	// Попытка подключиться внутри кластера
	config, err = rest.InClusterConfig()
	if err != nil {
		// Fallback: использование kubeconfig для разработки
		kubeconfig := flag.String("kubeconfig", "", "path to kubeconfig")
		flag.Parse()
		
		if *kubeconfig != "" {
			config, err = clientcmd.BuildConfigFromFlags("", *kubeconfig)
		} else {
			config, err = clientcmd.BuildConfigFromFlags("", os.Getenv("KUBECONFIG"))
		}
		if err != nil {
			return nil, fmt.Errorf("failed to build config: %v", err)
		}
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create clientset: %v", err)
	}

	return &PodHealer{
		clientset: clientset,
	}, nil
}

func (h *PodHealer) isPodStuck(pod *corev1.Pod) bool {
	// Pod в Pending состоянии больше 15 минут
	if pod.Status.Phase == corev1.PodPending {
		pendingDuration := time.Since(pod.CreationTimestamp.Time)
		if pendingDuration > 15*time.Minute {
			klog.Infof("Pod %s/%s stuck in Pending for %v", 
				pod.Namespace, pod.Name, pendingDuration)
			return true
		}
	}

	// Pod в CrashLoopBackOff
	if pod.Status.Phase == corev1.PodRunning {
		for _, containerStatus := range pod.Status.ContainerStatuses {
			if containerStatus.RestartCount > 10 {
				klog.Infof("Pod %s/%s in CrashLoopBackOff with %d restarts", 
					pod.Namespace, pod.Name, containerStatus.RestartCount)
				return true
			}
			
			// Проверяем состояние контейнера
			if containerStatus.State.Waiting != nil {
				if containerStatus.State.Waiting.Reason == "CrashLoopBackOff" {
					klog.Infof("Pod %s/%s container %s in CrashLoopBackOff", 
						pod.Namespace, pod.Name, containerStatus.Name)
					return true
				}
			}
		}
	}

	// Pod не Ready больше 10 минут
	if !isPodReady(pod) {
		for _, condition := range pod.Status.Conditions {
			if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionFalse {
				if time.Since(condition.LastTransitionTime.Time) > 10*time.Minute {
					klog.Infof("Pod %s/%s not ready for %v", 
						pod.Namespace, pod.Name, time.Since(condition.LastTransitionTime.Time))
					return true
				}
			}
		}
	}

	return false
}

func isPodReady(pod *corev1.Pod) bool {
	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

func (h *PodHealer) healPod(pod *corev1.Pod) error {
	klog.Infof("Attempting to heal pod %s/%s", pod.Namespace, pod.Name)
	
	// Проверяем аннотации для кастомного поведения
	if pod.Annotations != nil {
		if healingAction, exists := pod.Annotations["healing.kubernetes.io/action"]; exists {
			switch healingAction {
			case "restart":
				klog.Infof("Performing custom restart action for pod %s/%s", pod.Namespace, pod.Name)
			case "delete":
				klog.Infof("Performing custom delete action for pod %s/%s", pod.Namespace, pod.Name)
			case "ignore":
				klog.Infof("Skipping healing for pod %s/%s due to ignore annotation", pod.Namespace, pod.Name)
				return nil
			}
		}
	}

	// Удаляем проблемный Pod
	err := h.clientset.CoreV1().Pods(pod.Namespace).Delete(
		context.TODO(), 
		pod.Name, 
		metav1.DeleteOptions{},
	)
	
	if err != nil {
		klog.Errorf("Failed to heal pod %s/%s: %v", pod.Namespace, pod.Name, err)
		return err
	}
	
	klog.Infof("Successfully healed pod %s/%s", pod.Namespace, pod.Name)
	return nil
}

func (h *PodHealer) Run() {
	klog.Info("Starting Pod Healer Operator...")

	// Создаем watcher для Pod'ов
	watchlist := cache.NewListWatchFromClient(
		h.clientset.CoreV1().RESTClient(),
		"pods",
		corev1.NamespaceAll,
		fields.Everything(),
	)

	_, controller := cache.NewInformer(
		watchlist,
		&corev1.Pod{},
		time.Second*30, // Resync period
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				pod := obj.(*corev1.Pod)
				h.handlePod(pod)
			},
			UpdateFunc: func(oldObj, newObj interface{}) {
				newPod := newObj.(*corev1.Pod)
				h.handlePod(newPod)
			},
		},
	)

	// Запускаем контроллер
	stop := make(chan struct{})
	defer close(stop)
	go controller.Run(stop)

	klog.Info("Pod Healer Operator is running...")
	select {} // Бесконечное ожидание
}

func (h *PodHealer) handlePod(pod *corev1.Pod) {
	// Игнорируем Pod'ы в namespaces kube-system
	if pod.Namespace == "kube-system" {
		return
	}

	// Игнорируем Pod'ы с аннотацией ignore
	if pod.Annotations != nil {
		if _, exists := pod.Annotations["healing.kubernetes.io/ignore"]; exists {
			return
		}
	}

	if h.isPodStuck(pod) {
		if err := h.healPod(pod); err != nil {
			klog.Errorf("Error healing pod %s/%s: %v", pod.Namespace, pod.Name, err)
		}
	}
}

func main() {
	klog.InitFlags(nil)
	flag.Parse()

	healer, err := NewPodHealer()
	if err != nil {
		klog.Fatalf("Failed to create pod healer: %v", err)
	}

	healer.Run()
}
