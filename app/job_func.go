package app

type jobFunc struct {
	name string
	t string
	f func() (interface{}, error)
	detail interface{}
}

// JobRunnable from simple function
func JobFunc(name string, t string, f func() (interface{}, error)) JobRunnable {
	return &jobFunc{
		name: name,
		t: t,
		f: f,
	}
}

func (f *jobFunc) Name() string {
	return f.name
}

func (f *jobFunc) Type() string {
	return f.t
}

func (f *jobFunc) Run(statusFunc func(string)) error {
	statusFunc("Running")
	result, err := f.f()
	f.detail = result
	return err
}

func (f *jobFunc) Detail() interface{} {
	return f.detail
}
