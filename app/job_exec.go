package app

import (
	"../vaas"

	"fmt"
	"log"
	"sync"
)

type ExecJob struct {
	execStream *ExecStream
	pending map[int]vaas.Slice
	completed int

	lines *LinesBuffer
	mu sync.Mutex
}

func NewExecJob(query *DBQuery, vector []*DBSeries, nframes int) *ExecJob {
	j := &ExecJob{lines: new(LinesBuffer)}

	// get series for output vnodes
	var outputSeries []*DBSeries
	query.Load()
	for _, l := range query.Outputs {
		for _, output := range l {
			if output.Type != vaas.NodeParent {
				continue
			}
			node := GetNode(output.NodeID)
			vn := GetOrCreateVNode(node, vector)
			vn.EnsureSeries()
			outputSeries = append(outputSeries, &DBSeries{Series: *vn.Series})
		}
	}

	// determine which slices we need to apply on
	sets := make([][]vaas.Slice, len(vector))
	for i, series := range vector {
		for _, item := range series.ListItems() {
			sets[i] = append(sets[i], item.Slice)
		}
	}
	bigSlices := SliceIntersection(sets)
	j.pending = make(map[int]vaas.Slice)
	for _, bigSlice := range bigSlices {
		for start := bigSlice.Start; start < bigSlice.End; start += nframes {
			end := start + nframes
			if end > bigSlice.End {
				end = bigSlice.End
			}
			slice := vaas.Slice{bigSlice.Segment, start, end}
			haveAll := true
			for _, series := range outputSeries {
				if series.GetItem(slice) == nil {
					haveAll = false
					break
				}
			}
			if haveAll {
				continue
			}
			j.pending[len(j.pending)] = slice
		}
	}

	// create the exec stream
	j.execStream = NewExecStream(query, vector, j.sample, 4, vaas.ExecOptions{}, j.callback)

	return j
}

func (j *ExecJob) sample() *vaas.Slice {
	j.mu.Lock()
	defer j.mu.Unlock()
	for id, slice := range j.pending {
		delete(j.pending, id)
		return &slice
	}
	return nil
}

func (j *ExecJob) callback(slice vaas.Slice, outputs [][]vaas.DataReader, err error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	for _, l := range outputs {
		for _, rd := range l {
			waitErr := rd.Wait()
			if err == nil && waitErr != nil {
				err = waitErr
			}
			rd.Close()
		}
	}
	if err != nil {
		j.lines.Append(fmt.Sprintf("error applying on slice %v: %v", slice, err))
		return
	}
	j.lines.Append(fmt.Sprintf("finished slice %v", slice))
}

func (j *ExecJob) Name() string {
	return fmt.Sprintf("Apply %s on %v", j.execStream.query.Name, Vector(j.execStream.vector))
}

func (j *ExecJob) Type() string {
	return "cmd"
}

func (j *ExecJob) Run(statusFunc func(string)) error {
	statusFunc("Running")
	log.Printf("[job %v] applying query on %d slices that need outputs", j.Name(), len(j.pending))
	j.execStream.Get(len(j.pending))
	j.execStream.Wait()
	return nil
}

func (e *ExecJob) Detail() interface{} {
	return e.lines.Get()
}
