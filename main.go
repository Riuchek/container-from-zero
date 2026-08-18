package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"time"
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
	fmt.Printf("running %v as main pid %d\n", os.Args[2:], os.Getpid())

	cmd := exec.Command("/proc/self/exe", append([]string{"child"}, os.Args[2:]...)...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags: syscall.CLONE_NEWUTS | syscall.CLONE_NEWPID | syscall.CLONE_NEWNS | syscall.CLONE_NEWNET,
		Credential: &syscall.Credential{
			Uid: 0,
			Gid: 0,
		},
	}

	// 1. Inicia o processo filho em background sem bloquear
	must(cmd.Start())

	// 2. O Host configura a rede apontando para o PID do processo filho recém-criado
	setupHostNetwork(cmd.Process.Pid)

	// 3. Aguarda o container finalizar
	must(cmd.Wait())
}

func child() {
	fmt.Printf("running %v as child pid %d\n", os.Args[2:], os.Getpid())

	cg()

	must(syscall.Sethostname([]byte("container")))
	must(syscall.Chroot("/home/riuchek/CONTAINER_ROOT"))
	must(syscall.Chdir("/"))
	must(syscall.Mount("proc", "proc", "proc", 0, ""))

	// Aguarda 100ms para garantir que o Host terminou de injetar o vethGuest
	time.Sleep(100 * time.Millisecond)

	// Configura a ponta interna do container
	setupContainerNetwork()

	cmd := exec.Command(os.Args[2], os.Args[3:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	must(cmd.Run())
}

func cg() {
	cgroup := "/sys/fs/cgroup/riuchek"

	must(os.MkdirAll(cgroup, 0755))
	must(os.WriteFile(filepath.Join(cgroup, "pids.max"), []byte("20"), 0o644))
	must(os.WriteFile(filepath.Join(cgroup, "cgroup.procs"), []byte(strconv.Itoa(os.Getpid())), 0o644))

	must(os.WriteFile(filepath.Join(cgroup, "cpu.max"), []byte("80000 100000"), 0o644))  //CPU
	must(os.WriteFile(filepath.Join(cgroup, "cpu.weight"), []byte("50"), 0o644))         //CPU weight
	must(os.WriteFile(filepath.Join(cgroup, "memory.max"), []byte("1073741824"), 0o644)) //memoria
	must(os.WriteFile(filepath.Join(cgroup, "memory.low"), []byte("524288000"), 0o644))  //memoria low
}

// Roda no HOST
func setupHostNetwork(pid int) {
	vethHost := fmt.Sprintf("veth%d", pid)
	vethGuest := fmt.Sprintf("veth-g%d", pid)

	// Cria o par veth no Host
	must(exec.Command("ip", "link", "add", vethHost, "type", "veth", "peer", "name", vethGuest).Run())
	// Envia a ponta guest para o namespace de rede do PID do filho
	must(exec.Command("ip", "link", "set", vethGuest, "netns", strconv.Itoa(pid)).Run())
	// Atribui IP na interface do Host e sobe o link
	must(exec.Command("ip", "addr", "add", "10.200.1.1/24", "dev", vethHost).Run())
	must(exec.Command("ip", "link", "set", vethHost, "up").Run())

	// Nat/Masquerade no Host para permitir internet (ajuste eth0 se necessário)
	exec.Command("sysctl", "-w", "net.ipv4.ip_forward=1").Run()
	exec.Command("iptables", "-t", "nat", "-A", "POSTROUTING", "-s", "10.200.1.0/24", "-j", "MASQUERADE").Run()
}

// Roda DENTRO do Container
func setupContainerNetwork() {
	// Procura pela interface veth que foi movida para cá (geralmente veth-g<PID>)
	// Nota: Dentro do container ativamos o 'lo' e a interface atribuída
	exec.Command("ip", "link", "set", "lo", "up").Run()

	// Procura qualquer interface que comece com 'veth' dentro do container e configura
	out, _ := exec.Command("ip", "-o", "link", "show").Output()
	_ = out // Opcional: pode rodar direto comandos específicos caso queira renomear

	// Como o nome enviado foi veth-g<PID>, podemos usar wildcard ou pegar o nome via ip link
	// Simplificado para auto-configuração:
	exec.Command("sh", "-c", "ip link set dev $(ip -o link show | grep veth | awk -F': ' '{print $2}') up").Run()
	exec.Command("sh", "-c", "ip addr add 10.200.1.2/24 dev $(ip -o link show | grep veth | awk -F': ' '{print $2}')").Run()
	exec.Command("ip", "route", "add", "default", "via", "10.200.1.1").Run()
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
