package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
)

func main() {
	switch os.Args[1] {
	case "run":
		run()
	case "child":
		child()
	default:
		panic("help")
	}
}

func run() {
	fmt.Printf("running %v as pid %d\n", os.Args[2:], os.Getpid())

	cmd := exec.Command("/proc/self/exe", append([]string{"child"}, os.Args[2:]...)...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags: syscall.CLONE_NEWUTS | syscall.CLONE_NEWPID | syscall.CLONE_NEWNS,
		Credential: &syscall.Credential{
			Uid: 0,
			Gid: 0,
		},
	}

	must(cmd.Run())
}

func child() {
	fmt.Printf("running %v as pid %d\n", os.Args[2:], os.Getpid())

	cg()
	cmd := exec.Command(os.Args[2], os.Args[3:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	must(syscall.Sethostname([]byte("container")))
	must(syscall.Chroot("/home/riuchek/CONTAINER_ROOT"))
	must(syscall.Chdir("/"))
	must(syscall.Mount("proc", "proc", "proc", 0, ""))

	must(cmd.Run())
}

func cg() {
	cgroup := "/sys/fs/cgroup/riuchek"

	must(os.MkdirAll(cgroup, 0755))
	must(os.WriteFile(filepath.Join(cgroup, "pids.max"), []byte("20"), 0o644))
	must(os.WriteFile(filepath.Join(cgroup, "cgroup.procs"), []byte(strconv.Itoa(os.Getpid())), 0o644))
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
