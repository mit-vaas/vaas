package app

import (
	"github.com/googollee/go-socket.io"
)

var SetupFuncs []func(*socketio.Server)

func checkErr(err error) {
	if err != nil {
		panic(err)
	}
}
