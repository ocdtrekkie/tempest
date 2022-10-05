// `sandstorm-sandbox-agent` runs inside the grain's sandbox, and is the first
// program executed during grain startup. Its file descriptor #3 is a socket
// over which we can speak capnp to the sandstorm server outside the sandbox.
//
// Any APIs available to the grain which don't actually need privileges the grain
// doesn't have should ideally be implemented here; this helps us minimize attack
// surface.
package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"os"
	"os/exec"
	"strconv"

	"capnproto.org/go/capnp/v3"
	"zenhack.net/go/sandstorm-next/go/internal/util"
	"zenhack.net/go/sandstorm/capnp/spk"
)

type Command struct {
	Args []string
	Env  []string
}

func (c Command) ToOsCmd() *exec.Cmd {
	ret := exec.Command(c.Args[0], c.Args[1:]...)
	ret.Env = c.Env
	return ret
}

func parseCmd(cmd spk.Manifest_Command) (Command, error) {
	argv, err := cmd.Argv()
	if err != nil {
		return Command{}, err
	}
	var args []string
	for i := 0; i < argv.Len(); i++ {
		arg, err := argv.At(i)
		if err != nil {
			return Command{}, err
		}
		args = append(args, arg)
	}
	if len(args) == 0 {
		return Command{}, fmt.Errorf("len(cmd.argv) == 0")
	}
	environ, err := cmd.Environ()
	if err != nil {
		return Command{}, err
	}
	var env []string
	for i := 0; i < environ.Len(); i++ {
		kv := environ.At(i)
		k, err := kv.Key()
		if err != nil {
			return Command{}, err
		}
		v, err := kv.Value()
		if err != nil {
			return Command{}, err
		}
		env = append(env, k+"="+v)
	}
	return Command{
		Args: args,
		Env:  env,
	}, nil

}

func main() {
	data, err := ioutil.ReadFile("/sandstorm-manifest")
	util.Chkfatal(err)
	msg, err := capnp.Unmarshal(data)
	util.Chkfatal(err)
	manifest, err := spk.ReadRootManifest(msg)
	util.Chkfatal(err)
	appTitle, err := manifest.AppTitle()
	util.Chkfatal(err)
	text, err := appTitle.DefaultText()
	util.Chkfatal(err)
	fmt.Println("App title: ", text)
	spkCmd, err := manifest.ContinueCommand()
	util.Chkfatal(err)
	cmd, err := parseCmd(spkCmd)
	util.Chkfatal(err)

	fmt.Printf("Command: %v", cmd.Args)
	if cmd.Args[0] != "/sandstorm-http-bridge" {
		panic("Only sandstorm-http-bridge apps are supported.")
	}
	if len(cmd.Args) < 4 {
		// should be like /sandstorm-http-bridge <port-no> -- /app/command ...args
		panic("Too few arugments")
	}
	portNo, err := strconv.Atoi(cmd.Args[1])
	util.Chkfatal(err)
	if cmd.Args[2] != "--" {
		panic("Error: second argument should be '--' separator.")
	}
	cmd.Args = cmd.Args[3:]
	osCmd := cmd.ToOsCmd()

	util.Chkfatal(startCapnpApi())

	util.Chkfatal(osCmd.Start())
	go func() {
		defer os.Exit(1)
		util.Chkfatal(osCmd.Wait())
		log.Println("App exited; shutting down grain.")
	}()

	log.Printf("App started on port #%v. TODO: connect to it and do stuff.", portNo)

	<-context.Background().Done()

	/*
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		fds, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_STREAM, 0)
		util.Chkfatal(err)
		parentSock := os.NewFile(uintptr(fds[0]), "Parent socket")
		defer parentSock.Close()
		childSock := os.NewFile(uintptr(fds[1]), "Child socket")
		cmd.ExtraFiles = []*os.File{childSock}
	*/
}

func startCapnpApi() error {
	l, err := net.Listen("unix", "/tmp/sandstorm-api")
	if err != nil {
		return err
	}
	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				continue
			}
			// TODO: do something with the connection.
			log.Println("Got a connection to the api socket.")
			conn.Close()
		}
	}()
	return nil
}