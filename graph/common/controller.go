package common

import (
	"net/http"

	controller "github.com/traceshield/trace-shield-controller/generated/client/clientset/versioned"
	ctrl "sigs.k8s.io/controller-runtime"
)

func NewControllerClient(httpClient *http.Client) (*controller.Clientset, error) {
	kubeconfig := ctrl.GetConfigOrDie()
	return controller.NewForConfigAndClient(kubeconfig, httpClient)
}
