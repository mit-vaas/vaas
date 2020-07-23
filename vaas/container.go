package vaas

type EnvSetID struct {
	// query or job
	Type string

	// either query or job ID depending on Type
	RefID int
}

type Environment struct {
	Template string
	Requirements map[string]int

	// e.g. the node ID
	RefID int
}

type EnvSet struct {
	ID EnvSetID
	Environments []Environment
}

type Machine struct {
	BaseURL string
	Resources map[string]int
}

type Container struct {
	UUID string
	Environment Environment
	BaseURL string
	MachineIdx int
}
