package mutate

import (
	"fmt"
	"os"
	"strings"

	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/yaml"
)

const (
	monitoringOptsArg = "monitoring-options"
	processImageArg   = "process-image"
)

// SidecarConfig for sidecar parameters.
type SidecarConfig struct {
	Volumes        []v1.Volume    `json:"volumes"`
	Containers     []v1.Container `json:"containers"`
	InitContainers []v1.Container `json:"initcontainers"`
}

type PatchOperation struct {
	Op    string      `json:"op"`
	Path  string      `json:"path"`
	Value interface{} `json:"value,omitempty"`
}

// Load loads common sidecar config from file, application specific config like monitoring process name and monitoring path
// from application pod annotations
func (sc *SidecarConfig) Load(configFile string, annotations map[string]string) error {
	data, err := os.ReadFile(configFile)
	if err != nil {
		return err
	}
	if err := yaml.Unmarshal(data, sc); err != nil {
		return err
	}
	sc.ConfigFromAnnotations(annotations)

	// add MinIO credentials
	sData, err := ReadMinIOSecret()
	if err != nil {
		return err
	}
	sc.addMinIOCredentials(sData)

	return nil
}

// CreatePatch creates mutation patch for pod
func (sc *SidecarConfig) CreatePatch(pod v1.Pod) ([]PatchOperation, error) {
	var patches []PatchOperation
	if sc != nil {
		patches = append(patches, addPatches(sc.InitContainers, pod.Spec.InitContainers, "/spec/initContainers")...)
		patches = append(patches, addPatches(sc.Containers, pod.Spec.Containers, "/spec/containers")...)
		patches = append(patches, addPatches(sc.Volumes, pod.Spec.Volumes, "/spec/volumes")...)
	}

	return patches, nil
}

func addPatches[T any](newCollection []T, existingCollection []T, path string) []PatchOperation {
	var patches []PatchOperation
	for index, item := range newCollection {
		indexPath := path
		var value interface{}
		first := index == 0 && len(existingCollection) == 0
		if !first {
			indexPath = indexPath + "/-"
			value = item

		} else {
			value = []T{item}
		}
		patches = append(patches, PatchOperation{
			Op:    "add",
			Path:  indexPath,
			Value: value,
		})
	}
	return patches
}

// ConfigFromAnnotations creates config from pod annotations
func (sc *SidecarConfig) ConfigFromAnnotations(annotations map[string]string) {
	for i := range sc.Containers {
		opts := make([]string, 0)
		for k, v := range annotations {
			if strings.HasSuffix(k, AnnotationMonitoringPaths) {
				procName := strings.TrimSuffix(k, fmt.Sprintf(".%s", AnnotationMonitoringPaths))
				if procName != "" {
					paths := strings.Split(v, ",")
					for pi, p := range paths {
						paths[pi] = strings.TrimSpace(p)
					}
					opts = append(opts, fmt.Sprintf("%s=%s", procName, strings.Join(paths, ",")))
				}
			}
		}
		sc.Containers[i].Args = append(sc.Containers[i].Args, fmt.Sprintf("--%s=%s", monitoringOptsArg, strings.Join(opts, " ")))

		if annotation, ok := annotations[AnnotationProcessImage]; ok {
			sc.Containers[i].Args = append(sc.Containers[i].Args, fmt.Sprintf("--%s=%s", processImageArg, annotation))
		}
	}
}

func (sc *SidecarConfig) addMinIOCredentials(in *MinIOSecretData) {
	if len(sc.Containers) == 0 {
		return
	}
	sc.Containers[0].Env = append(sc.Containers[0].Env, v1.EnvVar{
		Name:  "MINIO_SERVER_USER",
		Value: in.UserName,
	})
	sc.Containers[0].Env = append(sc.Containers[0].Env, v1.EnvVar{
		Name:  "MINIO_SERVER_PASSWORD",
		Value: in.UserPassword,
	})
}
