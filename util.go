package master

import (
	"net"
	"os"
	"runtime"
)

var (
	booted  bool
	masterE *master
	workerE *worker
)

func Listeners(resolveAddrFunc func() []string) []*Listener {
	if booted {
		panic("this package is only allowed to be called once")
	}
	booted = true

	bootListeners := getParentListeners()
	//非linux系统视当前进程为worker
	if runtime.GOOS != "linux" {
		var (
			err       error
			addresses = resolveAddrFunc()
		)
		if len(addresses) == 0 {
			panic("getAddrFunc resolve empty addr")
		}
		if bootListeners, err = createListeners(addresses); err != nil {
			panic(err)
		}
	}

	if len(bootListeners) == 0 {
		masterE = newMaster(resolveAddrFunc)
		masterE.run()
	} else {
		workerE = newWorker(bootListeners)
		go workerE.run()
	}
	return workerE.listeners()
}

func Wait() {
	if workerE != nil {
		workerE.wait()
	}
}

func RegisterExitEvent(event func()) {
	if workerE != nil {
		workerE.registerExitEvent(event)
	}
}

func createListeners(addresses []string) (lns []net.Listener, err error) {
	var (
		ln       net.Listener
		iterator *net.TCPAddr
		tcpAddr  []*net.TCPAddr
	)
	defer func() {
		if err != nil {
			for _, ln = range lns {
				_ = ln.Close()
			}
		}
	}()

	for _, v := range addresses {
		if iterator, err = net.ResolveTCPAddr("tcp", v); err != nil {
			return nil, err
		}
		tcpAddr = append(tcpAddr, iterator)
	}

	for _, addr := range tcpAddr {
		if ln, err = net.ListenTCP("tcp", addr); err != nil {
			return nil, err
		}
		lns = append(lns, ln)
	}
	return
}

func getParentListeners() (lns []net.Listener) {
	var (
		err  error
		file *os.File
		ln   net.Listener
	)

	for i := 3; ; i++ {
		file = os.NewFile(uintptr(i), "")
		if ln, err = net.FileListener(file); err != nil {
			break
		}
		lns = append(lns, ln)
	}
	return
}
