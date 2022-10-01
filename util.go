package master

import (
	"net"
	"os"
	"runtime"
)

var (
	booted       bool
	workerEntity *worker
)

func Listeners(resolveAddrFunc func() []string) []*Listener {
	if booted {
		panic("this package is only allowed to be called once")
	}
	booted = true

	var (
		err       error
		listeners = getParentListeners()
	)

	if len(listeners) == 0 && runtime.GOOS == "linux" {
		masterEntity := newMaster(resolveAddrFunc)
		masterEntity.run()
	}

	if len(listeners) == 0 {
		addresses := resolveAddrFunc()
		if len(addresses) == 0 {
			panic("getAddrFunc resolve empty addr")
		}
		if listeners, err = createListenersWithAddr(addresses); err != nil {
			panic(err)
		}
	}
	workerEntity = newWorker(listeners)
	go workerEntity.run()

	return workerEntity.listeners()
}

func Wait() {
	if workerEntity != nil {
		workerEntity.waitQuit()
	}
}

func RegisterExitEvent(event func()) {
	if workerEntity != nil {
		workerEntity.registerExitEvent(event)
	}
}

func createListenersWithAddr(addresses []string) (lns []*Listener, err error) {
	var (
		ln      net.Listener
		tcpAddr *net.TCPAddr
	)
	defer func() {
		if err != nil {
			for _, ln = range lns {
				_ = ln.Close()
			}
		}
	}()
	for _, v := range addresses {
		if tcpAddr, err = net.ResolveTCPAddr("tcp", v); err != nil {
			return nil, err
		}
		if ln, err = net.ListenTCP("tcp", tcpAddr); err != nil {
			return nil, err
		}
		lns = append(lns, &Listener{Listener: ln})
	}
	return
}

func getParentListeners() (lns []*Listener) {
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
		lns = append(lns, &Listener{Listener: ln, file: file})
	}
	return
}
