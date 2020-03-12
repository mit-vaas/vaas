package main

import (
	"bufio"
	"encoding/json"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"strings"
)

func readTextFile(fname string) string {
	bytes, err := ioutil.ReadFile(fname)
	if err != nil {
		panic(err)
	}
	return string(bytes)
}

func readJSONFile(fname string, res interface{}) {
	bytes, err := ioutil.ReadFile(fname)
	if err != nil {
		panic(err)
	}
	if err := json.Unmarshal(bytes, res); err != nil {
		panic(err)
	}
}

func checkErr(err error) {
	if err != nil {
		panic(err)
	}
}

func jsonResponse(w http.ResponseWriter, x interface{}) {
	bytes, err := json.Marshal(x)
	if err != nil {
		panic(err)
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(bytes)
}

func jsonRequest(w http.ResponseWriter, r *http.Request, x interface{}) error {
	bytes, err := ioutil.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(400)
		return err
	}
	if err := json.Unmarshal(bytes, x); err != nil {
		w.WriteHeader(400)
		return err
	}
	return nil
}

func printStderr(prefix string, stderr io.ReadCloser) {
	rd := bufio.NewReader(stderr)
	for {
		line, err := rd.ReadString('\n')
		if err == io.EOF {
			break
		} else if err != nil {
			panic(err)
		}
		if Debug {
			log.Printf("[" + prefix + "] " + strings.TrimSpace(line))
		}
	}
}

func mod(a, b int) int {
	x := a%b
	if x < 0 {
		x = x+b
	}
	return x
}
