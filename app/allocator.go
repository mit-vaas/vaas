package app

import (
	"../vaas"

	"fmt"
	"log"
	"net/http"
	"net/url"
	"sync"
)

type machines struct {
	machines []vaas.Machine
	mu sync.Mutex
}

var Machines *machines = &machines{}

func (m *machines) GetList() []vaas.Machine {
	m.mu.Lock()
	defer m.mu.Unlock()
	var machines []vaas.Machine
	for _, machine := range m.machines {
		machines = append(machines, machine)
	}
	return machines
}

func (m *machines) Register(machine vaas.Machine) {
	m.mu.Lock()
	m.machines = append(m.machines, machine)
	m.mu.Unlock()
}

// An Allocator allocates environments onto containers, and assigns containers
// when callers need to use those environments.
type Allocator interface {
	// assign a container for each environment needed by the caller
	// returns nil if the env isn't allocated
	Pick(vaas.EnvSetID) []vaas.Container

	// allocate new containers (if needed) for the environment
	Allocate(vaas.EnvSet) []vaas.Container

	// de-allocate an entire environment set
	// either when job finished or query is updated
	Deallocate(vaas.EnvSetID)

	GetEnvSets() []vaas.EnvSetID

	// returns all containers allocated for this env
	// does not allocate any new containers
	GetContainers(vaas.EnvSetID) [][]vaas.Container
}

// Allocate the minimum number of containers to satisfy the requested environment sets.
type MinimalAllocator struct {
	envSets map[vaas.EnvSetID]vaas.EnvSet
	// minimal allocator puts exactly one container for each environment in the EnvSet
	containers map[vaas.EnvSetID][]vaas.Container
	mu sync.Mutex
}

var allocator = &SmartAllocator{
	envSets: make(map[vaas.EnvSetID]vaas.EnvSet),
	containers: make(map[vaas.EnvSetID][][]vaas.Container),
	roundRobinIdx: make(map[vaas.EnvSetID][]int),
}

func GetAllocator() Allocator {
	return allocator
}

func (a *MinimalAllocator) FlatContainers() []vaas.Container {
	var containers []vaas.Container
	for _, l := range a.containers {
		for _, c := range l {
			containers = append(containers, c)
		}
	}
	return containers
}

func (a *MinimalAllocator) Allocate(set vaas.EnvSet) []vaas.Container {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.containers[set.ID] != nil {
		return a.containers[set.ID]
	}

	// if we can fit it without de-allocating anyone, then let's do that
	if ok := a.tryAllocate(set); ok {
		return a.containers[set.ID]
	}

	panic(fmt.Errorf("TODO: implement de-allocation"))
}

// caller must have the lock
func (a *MinimalAllocator) tryAllocate(set vaas.EnvSet) bool {
	// try to fit the envset, return false if it's not possible
	// greedily prefer machines where we've already allocated other containers in this set
	machines := Machines.GetList()
	machineHits := make(map[int]int)
	machineUsage := make([]map[string]int, len(machines))
	for i, machine := range machines {
		machineUsage[i] = make(map[string]int)
		for k, v := range machine.Resources {
			machineUsage[i][k] = v
		}
	}
	for _, container := range a.FlatContainers() {
		for k, v := range container.Environment.Requirements {
			machineUsage[container.MachineIdx][k] -= v
		}
	}
	var allocation []int
	for _, env := range set.Environments {
		var bestMachineIdx int = -1
		for i, usage := range machineUsage {
			ok := true
			for k, v := range env.Requirements {
				if usage[k] < v {
					ok = false
					break
				}
			}
			if !ok {
				continue
			}
			if bestMachineIdx == -1 || machineHits[i] > machineHits[bestMachineIdx] {
				bestMachineIdx = i
			}
		}
		if bestMachineIdx == -1 {
			return false
		}
		for k, v := range env.Requirements {
			machineUsage[bestMachineIdx][k] -= v
		}
		allocation = append(allocation, bestMachineIdx)
	}

	// we were able to fit it, so now we need to actually allocate on each machine and collect the Containers
	var containers []vaas.Container
	for envIdx, env := range set.Environments {
		machine := machines[allocation[envIdx]]
		var container vaas.Container
		err := vaas.JsonPost(machine.BaseURL, "/allocate", env, &container)
		if err != nil {
			panic(fmt.Errorf("allocation error: %v", err))
		}
		container.Environment = env
		container.MachineIdx = allocation[envIdx]
		containers = append(containers, container)
	}
	a.envSets[set.ID] = set
	a.containers[set.ID] = containers

	return true
}

func (a *MinimalAllocator) Deallocate(setID vaas.EnvSetID) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.containers[setID] == nil {
		return
	}
	for _, container := range a.containers[setID] {
		log.Printf("[allocator] begin de-allocating container %s", container.UUID)
		resp, err := http.PostForm(Machines.GetList()[container.MachineIdx].BaseURL + "/deallocate", url.Values{"uuid": {container.UUID}})
		if err != nil {
			panic(fmt.Errorf("de-allocation error: %v", err))
		} else if resp.StatusCode != 200 {
			panic(fmt.Errorf("de-allocation error: got status code %v", resp.StatusCode))
		}
		resp.Body.Close()
		log.Printf("[allocator] successfully de-allocated container %s", container.UUID)
	}
	delete(a.envSets, setID)
	delete(a.containers, setID)
}

func (a *MinimalAllocator) GetEnvSets() []vaas.EnvSetID {
	a.mu.Lock()
	defer a.mu.Unlock()
	var ids []vaas.EnvSetID
	for id := range a.envSets {
		ids = append(ids, id)
	}
	return ids
}

func (a *MinimalAllocator) GetContainers(setID vaas.EnvSetID) [][]vaas.Container {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.containers[setID] == nil {
		return nil
	}
	containers := make([][]vaas.Container, len(a.containers[setID]))
	for i, container := range a.containers[setID] {
		containers[i] = []vaas.Container{container}
	}
	return containers
}

func init() {
	QueryChangeListeners = append(QueryChangeListeners, func(query *DBQuery) {
		allocator.Deallocate(vaas.EnvSetID{"query", query.ID})
	})

	http.HandleFunc("/register-machine", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			w.WriteHeader(404)
			return
		}
		var machine vaas.Machine
		if err := vaas.ParseJsonRequest(w, r, &machine); err != nil {
			return
		}
		Machines.Register(machine)
	})
}
