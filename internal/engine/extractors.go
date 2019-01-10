package engine

import "github.com/windmilleng/tilt/internal/model"

// Extract the targets that we can apply, or nil if we can't apply these targets.
func extractImageAndK8sTargets(specs []model.TargetSpec) (images []model.ImageTarget, resources []model.K8sTarget) {
	for _, s := range specs {
		switch s := s.(type) {
		case model.ImageTarget:
			images = append(images, s)
		case model.K8sTarget:
			resources = append(resources, s)
		default:
			return nil, nil
		}
	}
	return images, resources
}
