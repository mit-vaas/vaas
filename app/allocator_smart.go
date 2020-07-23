package app

import (
	"../vaas"

	"fmt"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"sync"
)

// Allocation strategy: divide resources evenly between the active queries.
// Automatically de-allocate queries that are idle for longer than 30 sec. (TODO)
// Within a query, balance the resources among environments based on the idle time.
// So initially, distribute resources evenly, but then shift resources from
// environments with high average idle time to those with low idle time.
type SmartAllocator struct {
	envSets map[vaas.EnvSetID]vaas.EnvSet
	containers map[vaas.EnvSetID][][]vaas.Container
	roundRobinIdx map[vaas.EnvSetID][]int
	mu sync.Mutex
}

// Caller must have the lock.
func (a *SmartAllocator) flatContainers() []vaas.Container {
	var containers []vaas.Container
	for _, setlist := range a.containers {
		for _, envlist := range setlist {
			for _, c := range envlist {
				containers = append(containers, c)
			}
		}
	}
	return containers
}

// Returns free resources per machine.
// Caller must have the lock.
func (a *SmartAllocator) machineFree() []map[string]int {
	free := make([]map[string]int, len(Machines))
	for i, machine := range Machines {
		free[i] = make(map[string]int)
		for k, v := range machine.Resources {
			free[i][k] = v
		}
	}
	for _, container := range a.flatContainers() {
		for k, v := range container.Environment.Requirements {
			free[container.MachineIdx][k] -= v
		}
	}
	return free
}

// Pick containers for an envSet in round-robin fashion.
// Caller must have the lock.
func (a *SmartAllocator) pick(set vaas.EnvSet) []vaas.Container {
	var containers []vaas.Container
	if a.roundRobinIdx[set.ID] == nil {
		a.roundRobinIdx[set.ID] = make([]int, len(set.Environments))
	}
	rrIdx := a.roundRobinIdx[set.ID]
	for envIdx := range set.Environments {
		envlist := a.containers[set.ID][envIdx]
		idx := rrIdx[envIdx]
		if idx >= len(envlist) {
			// could happen after some de-allocations
			idx = 0
		}
		containers = append(containers, envlist[idx])
		rrIdx[envIdx] = (idx + 1) % len(envlist)
	}
	return containers
}

func (a *SmartAllocator) Allocate(set vaas.EnvSet) []vaas.Container {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.containers[set.ID] != nil {
		return a.pick(set)
	}

	// find what an even division of the resources between envsets is
	setResources := make(map[string]int)
	for _, machine := range Machines {
		for k, v := range machine.Resources {
			setResources[k] += v
		}
	}
	for k := range setResources {
		setResources[k] /= len(a.envSets)+1
	}

	// de-allocate pre-existing allocations until they are at most the even division
	for setID, setlist := range a.containers {
		// get flat list of containers
		var setContainers []vaas.Container
		for _, envlist := range setlist {
			for _, c := range envlist {
				setContainers = append(setContainers, c)
			}
		}

		allocResources := make(map[string]int)
		var destroyContainers []vaas.Container
		for _, idx := range rand.Perm(len(setContainers)) {
			container := setContainers[idx]
			for k, v := range container.Environment.Requirements {
				if allocResources[k]+v <= setResources[k] {
					allocResources[k] += v
					continue
				}
				destroyContainers = append(destroyContainers, container)
				break
			}
		}
		for _, container := range destroyContainers {
			a.deallocate(setID, container)
		}
	}

	// allocate containers for this set evenly among its environments until no more
	// resources are left
	a.envSets[set.ID] = set
	a.containers[set.ID] = make([][]vaas.Container, len(set.Environments))
	var envIdx int = 0
	for {
		env := set.Environments[envIdx]
		free := a.machineFree()
		var machineIdx int = -1
		for i, free := range free {
			ok := true
			for k, v := range env.Requirements {
				if free[k] < v {
					ok = false
					break
				}
			}
			if !ok {
				continue
			}
			machineIdx = i
			break
		}
		if machineIdx == -1 {
			break
		}

		// perform the allocation
		log.Printf("[allocator] [set %v] allocating env template=%s on machine %d", set.ID, env.Template, machineIdx)
		var container vaas.Container
		err := vaas.JsonPost(Machines[machineIdx].BaseURL, "/allocate", env, &container)
		if err != nil {
			panic(fmt.Errorf("allocation error: %v", err))
		}
		container.Environment = env
		container.MachineIdx = machineIdx
		a.containers[set.ID][envIdx] = append(a.containers[set.ID][envIdx], container)
		envIdx = (envIdx+1) % len(set.Environments)
	}

	for envIdx, envlist := range a.containers[set.ID] {
		if len(envlist) == 0 {
			panic(fmt.Errorf("failed to allocate containers for set %v env %d", set.ID, envIdx))
		}
	}

	return a.pick(set)
}

// caller must have lock
func (a *SmartAllocator) deallocate(setID vaas.EnvSetID, container vaas.Container) {
	log.Printf("[allocator] begin de-allocating container %s", container.UUID)
	resp, err := http.PostForm(Machines[container.MachineIdx].BaseURL + "/deallocate", url.Values{"uuid": {container.UUID}})
	if err != nil {
		panic(fmt.Errorf("de-allocation error: %v", err))
	} else if resp.StatusCode != 200 {
		panic(fmt.Errorf("de-allocation error: got status code %v", resp.StatusCode))
	}
	resp.Body.Close()
	log.Printf("[allocator] successfully de-allocated container %s", container.UUID)

	newContainers := make([][]vaas.Container, len(a.containers[setID]))
	for envIdx, envlist := range a.containers[setID] {
		for _, c := range envlist {
			if c.UUID == container.UUID {
				continue
			}
			newContainers[envIdx] = append(newContainers[envIdx], c)
		}
	}
	a.containers[setID] = newContainers
}

func (a *SmartAllocator) Deallocate(setID vaas.EnvSetID) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.containers[setID] == nil {
		return
	}
	for _, envlist := range a.containers[setID] {
		for _, c := range envlist {
			a.deallocate(setID, c)
		}
	}
	delete(a.envSets, setID)
	delete(a.containers, setID)
	delete(a.roundRobinIdx, setID)
}

func (a *SmartAllocator) GetEnvSets() []vaas.EnvSetID {
	a.mu.Lock()
	defer a.mu.Unlock()
	var ids []vaas.EnvSetID
	for id := range a.envSets {
		ids = append(ids, id)
	}
	return ids
}

func (a *SmartAllocator) GetContainers(setID vaas.EnvSetID) [][]vaas.Container {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.containers[setID] == nil {
		return nil
	}
	containers := make([][]vaas.Container, len(a.containers[setID]))
	for envIdx, envlist := range a.containers[setID] {
		for _, c := range envlist {
			containers[envIdx] = append(containers[envIdx], c)
		}
	}
	return containers
}
