package microservice

import (
	appv1 "KubeService/pkg/apis/app/v1"
	"context"
	v1 "k8s.io/api/core/v1"
	extensionsv1beta1 "k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"reflect"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func (r *ReconcileMicroService) reconcileLoadBalance(microService *appv1.MicroService) error {
	lb := microService.Spec.LoadBalance

	if len(microService.Spec.Versions) == 0 {
		return nil
	}

	currentVersion := microService.Spec.Versions[0]
	for _, version := range microService.Spec.Versions {
		if version.Name == microService.Spec.CurrentVersionName {
			currentVersion = version
			break
		}
	}

	if reflect.ValueOf(lb).IsNil() {
		// init default LB
		lb = appv1.LoadBalance{}
	}

	svcLB := lb.Service
	enableSVC := false
	if !reflect.ValueOf(svcLB).IsNil() {
		// If use define custom Service
		enableSVC = true
		if svcLB.Spec.Selector == nil {
			svcLB.Spec.Selector = currentVersion.Template.Selector.MatchLabels
		}
		svc, err := makeService(microService.Name, microService.Namespace, microService.Labels, &svcLB.Spec)
		if err != nil {
			return err
		}
		svc.Labels = microService.Labels
		if err := controllerutil.SetControllerReference(microService, svc, r.scheme); err != nil {
			return err
		}

		if err := r.updateOrCreateSvc(svc); err != nil {
			return err
		}
	}

	ingressLB := lb.Ingress
	if !reflect.ValueOf(ingressLB).IsNil() {
		ingress := &extensionsv1beta1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name:      ingressLB.Name,
				Namespace: microService.Namespace,
				Labels:    microService.Labels,
			},
			Spec: ingressLB.Spec,
		}
		found := &extensionsv1beta1.Ingress{}
		err := r.Get(context.TODO(), types.NamespacedName{Name: ingress.Name, Namespace: ingress.Namespace}, found)
		if err != nil && errors.IsNotFound(err) {
			log.Info("Creating Ingress", "namespace", ingress.Namespace, "name", ingress.Name)
			err = r.Create(context.TODO(), ingress)
			return err
		} else if err != nil {
			return err
		}

		// Update the found object and write the result back if there are any changes
		if !reflect.DeepEqual(ingress.Spec, found.Spec) {
			found.Spec = ingress.Spec
			log.Info("Updating Ingress", "namespace", ingress.Namespace, "name", ingress.Name)
			err = r.Update(context.TODO(), found)
			if err != nil {
				return err
			}
		}
	}

	if !enableSVC {
		return nil
	}

	for _, version := range microService.Spec.Versions {
		spec := svcLB.Spec.DeepCopy()
		spec.Selector = version.Template.Selector.MatchLabels
		svc, err := makeService(microService.Name+"-"+version.Name, microService.Namespace, microService.Labels, spec)
		if err != nil {
			return err
		}
		if err := controllerutil.SetControllerReference(microService, svc, r.scheme); err != nil {
			return err
		}

		if err := r.updateOrCreateSvc(svc); err != nil {
			return err
		}
	}

	return nil
}

func (r *ReconcileMicroService) updateOrCreateSvc(svc *v1.Service) error {
	// Check if the Service already exists
	found := &v1.Service{}
	err := r.Get(context.TODO(), types.NamespacedName{Name: svc.Name, Namespace: svc.Namespace}, found)
	if err != nil && errors.IsNotFound(err) {
		log.Info("Creating Service", "namespace", svc.Namespace, "name", svc.Name)
		err = r.Create(context.TODO(), svc)
		return err
	} else if err != nil {
		return err
	}

	// Update the found object and write the result back if there are any changes
	if !reflect.DeepEqual(svc.Spec, found.Spec) {
		found.Spec = svc.Spec
		log.Info("Updating Service", "namespace", svc.Namespace, "name", svc.Name)
		err = r.Update(context.TODO(), found)
		if err != nil {
			return err
		}
	}
	return nil
}

func makeService(name string, namespace string, label map[string]string, svcSpec *v1.ServiceSpec) (*v1.Service, error) {
	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    label,
		},
		Spec: *svcSpec,
	}
	return svc, nil
}