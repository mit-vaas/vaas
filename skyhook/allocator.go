package skyhook

import (
	"fmt"
	"sync"
)

type EnvSetID struct {
	// query or job
	Type string

	// either query or job ID depending on Type
	RefID int
}

type Environment struct {
	Template string
	Requirements map[string]int
}

type EnvSet struct {
	ID EnvSetID
	Environments []Environment
}

type Machine struct {
	BaseURL string
	Resources map[string]int
}

var Machines = []Machine{
	Machine{"http://localhost:8081", map[string]int{"gpu": 2, "container": 8}},
}

type Container struct {
	Environment Environment
	BaseURL string
	MachineIdx int
}

// An Allocator allocates environments onto containers, and assigns containers
// when callers need to use those environments.
type Allocator interface {
	// assign a container for each environment needed by the caller
	GetContainers(EnvSet) []Container

	// de-allocate an entire environment set
	// either when job finished or query is updated
	Deallocate(EnvSetID)
}

// Allocate the minimum number of containers to satisfy the requested environment sets.
type MinimalAllocator struct {
	envSets map[EnvSetID]EnvSet
	// minimal allocator puts exactly one container for each environment in the EnvSet
	containers map[EnvSetID][]Container
	mu sync.Mutex
}

var allocator Allocator = &MinimalAllocator{
	envSets: make(map[EnvSetID]EnvSet),
	containers: make(map[EnvSetID][]Container),
}

func (a *MinimalAllocator) FlatContainers() []Container {
	var containers []Container
	for _, l := range a.containers {
		for _, c := range l {
			containers = append(containers, c)
		}
	}
	return containers
}

func (a *MinimalAllocator) GetContainers(set EnvSet) []Container {
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
func (a *MinimalAllocator) tryAllocate(set EnvSet) bool {
	// try to fit the envset, return false if it's not possible
	// greedily prefer machines where we've already allocated other containers in this set
	machineHits := make(map[int]int)
	machineUsage := make([]map[string]int, len(Machines))
	for i, machine := range Machines {
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
	var containers []Container
	for envIdx, env := range set.Environments {
		machine := Machines[allocation[envIdx]]
		var containerURL string
		err := JsonPost(machine.BaseURL, "/allocate", env, &containerURL)
		if err != nil {
			panic(fmt.Errorf("allocation error: %v", err))
		}
		containers = append(containers, Container{
			Environment: env,
			BaseURL: containerURL,
			MachineIdx: allocation[envIdx],
		})
	}
	a.containers[set.ID] = containers

	return true
}

func (a *MinimalAllocator) Deallocate(setID EnvSetID) {
	// TODO
}

/*
Transformer:
- Inputs a query graph and outputs another
- Each query specifies a sequence of transformers that should be applied one after another
- Some transformers may take a long time to apply
	e.g. they need to train an approximate model
	in this case the transformer should first return the graph unchanged
	start a job to train approximate model in the background
	and when that finishes it can notify the system to de-allocate the query graph so that the transformers are re-applied
- Transformer has per-vector configuration stored in a vtransformers table (like vnodes)

Distributed execution:
- The coordinator maintains a map: envID -> set of containers/executors where that env is allocated
- Nodes in query graph are annotated with environment IDs
	All nodes in the default env are allocated together
		So if we allocate one of them on container X => put the rest on X too
		This means their executor sets will be identical
	Other nodes are always allocated separately (different envs)

Allocation strategy:
- There is Allocator object that handles allocation of nodes/envs
	and I guess it notifies machines when they should create/destroy a container and which nodes should run in the container
type Allocator interface {
	// allocator maintains the executor map
	// this returns an executor for each node in the specified query graph in round-robin order
	// if it returns a specific container X for a node in default container, it returns X for all the rest too
	//GetExecutors(query *Query) map[int]Executor
	GetExecutor(env Environment) Executor

	// also need a function for jobs that require a constant amount of resources
}
- Allocator also keeps track of the idle percentage time, like if it is 20% of the time not doing any tasks
	maybe there are two numbers for interactive case, since otherwise it'd be dominated by the idle time due to nothing being requested
	or maybe the second number can be one used for long-term allocation strategy
		like it should be weighted by the # of containers that the node has been allocated on
	I guess these stats are stored per unique container in the query
		(all containers except default are unique)
- what to do if query isn't already allocated?
	(1) if there is an allocated query that has not been used for 10 sec, then de-allocate it
	(2) try to allocate on unallocated machines
	(3) if that doesn't work, try to replace extra containers with highest idle time
		extra meaning it's not first in the list of containers for the env
	(4) if that doesn't work, should we replace any idle container? or wait until a query becomes idle for 10 sec?
		depends on whether this is interactive I guess
		interactive should have priority
- rebalancing
	should be somehow based on the idle times
	for now the rebalancing can divide the resources evenly
		one resource is "container" which specifies maximum # containers a machine is willing to host
		another is "gpu" the # GPUs that machine has
		need to take this into account when allocating too

Query execution
- Get list of containers corresponding to the different environments in the query
	The Allocator should greedily assign environments on the same machine as envs that were assigned earlier in the query
		this overrides the round-robin part
- Assign all containers in the query, but have run function like we do now but nodes should also be able to call run function
	This run function determines which buffers we should actually initialize
	So a node should have a Run function or something where it decides what parents to run and in what order
- Basically this should support the implementation of reference nodes and short-circuit AND/OR nodes
	also MIRIS where it conditionally runs a sequence like video->detector->filter detections-> on a subset of frames, potentially repeatedly...
*/
