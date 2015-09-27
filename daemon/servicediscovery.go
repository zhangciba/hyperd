package daemon

import (
	"encoding/json"
	"fmt"
	"path"

	"github.com/hyperhq/hyper/engine"
	"github.com/hyperhq/hyper/servicediscovery"
	"github.com/hyperhq/hyper/utils"
	"github.com/hyperhq/runv/hypervisor"
	"github.com/hyperhq/runv/hypervisor/pod"
	"github.com/hyperhq/runv/lib/glog"
)

func (daemon *Daemon) AddService(job *engine.Job) error {
	var srv pod.UserService

	podId := job.Args[0]
	data := job.Args[1]

	vm, container, err := daemon.GetServiceContainerInfo(podId)
	if err != nil {
		return err
	}

	services, err := servicediscovery.GetServices(vm, container)
	if err != nil {
		return err
	}

	json.Unmarshal([]byte(data), &srv)
	services = append(services, srv)

	return servicediscovery.ApplyServices(vm, container, services)
}

func (daemon *Daemon) DeleteService(job *engine.Job) error {
	var srv pod.UserService
	var services []pod.UserService
	var services2 []pod.UserService
	var found int = 0

	podId := job.Args[0]
	data := job.Args[1]

	vm, container, err := daemon.GetServiceContainerInfo(podId)
	if err != nil {
		return err
	}

	services, err = servicediscovery.GetServices(vm, container)
	if err != nil {
		return err
	}

	json.Unmarshal([]byte(data), &srv)
	for _, s := range services {
		if s.ServicePort != srv.ServicePort {
			services2 = append(services2, s)
			continue
		}

		if s.ServiceIP != srv.ServiceIP {
			services2 = append(services2, s)
			continue
		}

		found = 1
		continue
	}

	if found == 0 {
		return fmt.Errorf("Pod %s doesn't container this service", podId)
	}

	return servicediscovery.ApplyServices(vm, container, services2)
}

func (daemon *Daemon) GetServices(job *engine.Job) error {
	podId := job.Args[0]

	vm, container, err := daemon.GetServiceContainerInfo(podId)
	if err != nil {
		return err
	}

	services, err := servicediscovery.GetServices(vm, container)
	if err != nil {
		return err
	}

	v := &engine.Env{}
	v.SetJson("data", services)
	if _, err := v.WriteTo(job.Stdout); err != nil {
		return err
	}

	return nil
}

func (daemon *Daemon) GetServiceContainerInfo(podId string) (*hypervisor.Vm, string, error) {
	daemon.PodsMutex.RLock()
	mypod, ok := daemon.PodList[podId]
	if !ok {
		daemon.PodsMutex.RUnlock()
		return nil, "", fmt.Errorf("Cannot find Pod %s", podId)
	}

	if mypod.Type != "service-discovery" || len(mypod.Containers) <= 1 {
		daemon.PodsMutex.RUnlock()
		return nil, "", fmt.Errorf("Pod %s doesn't have services discovery", podId)
	}

	container := mypod.Containers[0].Id
	vmId := mypod.Vm
	glog.V(1).Infof("Get container id is %s, vm %s", container, vmId)
	daemon.PodsMutex.RUnlock()

	vm, ok := daemon.VmList[vmId]
	if !ok {
		return nil, "", fmt.Errorf("Can find VM whose Id is %s!", vmId)
	}

	return vm, container, nil
}

func (daemon *Daemon) ProcessPodBytes(body []byte, podId string) (*pod.UserPod, error) {
	var containers []pod.UserContainer
	var serviceDir string = path.Join(utils.HYPER_ROOT, "services", podId)

	userPod, err := pod.ProcessPodBytes(body)
	if err != nil {
		glog.V(1).Infof("Process POD file error: %s", err.Error())
		return nil, err
	}

	if len(userPod.Services) == 0 {
		return userPod, nil
	}

	userPod.Type = "service-discovery"
	serviceContainer := pod.UserContainer{
		Name:  userPod.Name + "-service-discovery",
		Image: servicediscovery.ServiceImage,
	}

	serviceVolRef := pod.UserVolumeReference{
		Volume:   "service-volume",
		Path:     servicediscovery.ServiceVolume,
		ReadOnly: false,
	}

	/* PrepareServices will check service volume */
	serviceVolume := pod.UserVolume{
		Name:   "service-volume",
		Source: serviceDir,
		Driver: "vfs",
	}

	userPod.Volumes = append(userPod.Volumes, serviceVolume)

	serviceContainer.Volumes = append(serviceContainer.Volumes, serviceVolRef)

	containers = append(containers, serviceContainer)

	for _, c := range userPod.Containers {
		containers = append(containers, c)
	}

	userPod.Containers = containers

	return userPod, nil
}
