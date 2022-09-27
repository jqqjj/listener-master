package master

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"sync"
	"syscall"
	"time"
)

var (
	booted          bool
	parentListeners []net.Listener
)

func init() {
	parentListeners = getParentListeners()
}

func Listeners(resolveAddrFunc func() []string) []*Listener {
	if booted {
		panic("this package is only allowed to be called once")
	}
	booted = true

	if runtime.GOOS != "linux" {
		var (
			err       error
			tcpAddr   []*net.TCPAddr
			lns       []net.Listener
			addresses = resolveAddrFunc()
		)

		if len(addresses) == 0 {
			panic("getAddrFunc resolve empty addr")
		}
		if tcpAddr, err = parseTCPAddress(addresses); err != nil {
			panic(err)
		}
		if lns, err = createListeners(tcpAddr); err != nil {
			panic(err)
		}
		return parseListeners(lns)
	}

	if len(parentListeners) == 0 {
		runMaster(resolveAddrFunc)
	} else {
		go runWorker()
	}
	return parseListeners(parentListeners)
}

func runMaster(resolveAddrFunc func() []string) {
	var (
		err       error
		worker    *exec.Cmd
		workers   sync.Map
		waitGroup sync.WaitGroup
		tcpAddr   []*net.TCPAddr
		listeners []net.Listener
		addresses = resolveAddrFunc()
	)

	if len(addresses) == 0 {
		panic("getAddrFunc resolve empty addr")
	}
	if tcpAddr, err = parseTCPAddress(addresses); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	if listeners, err = createListeners(tcpAddr); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	if worker, err = createWorker(listeners); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	waitGroup.Add(1)
	workers.Store(worker, nil)
	go func(worker *exec.Cmd) {
		defer waitGroup.Done()
		defer workers.Delete(worker)
		if err = worker.Run(); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
	}(worker)

	sig := make(chan os.Signal)
	signal.Notify(sig, syscall.SIGHUP)
	signal.Notify(sig, syscall.SIGINT)
	signal.Notify(sig, syscall.SIGTERM)
	for v := range sig {
		if v.(syscall.Signal) == syscall.SIGINT || v.(syscall.Signal) == syscall.SIGTERM {
			go func() {
				for i := 0; i <= 3; i++ {
					workers.Range(func(key, value any) bool {
						if i == 3 {
							_ = key.(*exec.Cmd).Process.Kill()
						} else {
							_ = key.(*exec.Cmd).Process.Signal(syscall.SIGTERM)
						}
						return true
					})
					time.Sleep(time.Second)
				}
			}()
			break
		}
		if v.(syscall.Signal) == syscall.SIGHUP {
			var (
				newTcpAddr    []*net.TCPAddr
				newListeners  []net.Listener
				diffListeners []net.Listener
				newWorker     *exec.Cmd
			)

			addresses = resolveAddrFunc()
			if len(addresses) == 0 {
				fmt.Println("address is empty")
				continue
			}
			if newTcpAddr, err = parseTCPAddress(addresses); err != nil {
				fmt.Println("address error")
				continue
			}
			if diffListeners, err = createListeners(addrDiff(newTcpAddr, tcpAddr)); err != nil {
				fmt.Println("fail to create listener:", err)
				continue
			}
			for _, addr := range newTcpAddr {
				for _, ln := range diffListeners {
					if ln.Addr().(*net.TCPAddr).IP.Equal(addr.IP) && ln.Addr().(*net.TCPAddr).Port == addr.Port {
						newListeners = append(newListeners, ln)
					}
				}
				for _, ln := range listeners {
					if ln.Addr().(*net.TCPAddr).IP.Equal(addr.IP) && ln.Addr().(*net.TCPAddr).Port == addr.Port {
						newListeners = append(newListeners, ln)
					}
				}
			}
			//创建worker
			if newWorker, err = createWorker(newListeners); err != nil {
				fmt.Println("fail to create worker:", err)
				for _, ln := range diffListeners {
					_ = ln.Close()
				}
				continue
			}
			//运行worker
			waitGroup.Add(1)
			workers.Store(newWorker, nil)
			if err = newWorker.Start(); err != nil {
				fmt.Println("fail to start worker:", err)
				workers.Delete(newWorker)
				waitGroup.Done()
				continue
			}
			go func(worker *exec.Cmd) {
				defer waitGroup.Done()
				defer workers.Delete(worker)
				worker.Wait()
			}(newWorker)

			//清理不使用的listener
			for _, ln := range listenersDiff(listeners, newListeners) {
				_ = ln.Close()
			}
			//保存最新的worker状态
			listeners = newListeners
			tcpAddr = newTcpAddr
			//请求优雅退出
			workers.Range(func(key, value any) bool {
				if key.(*exec.Cmd) != newWorker {
					_ = key.(*exec.Cmd).Process.Signal(syscall.SIGHUP)
				}
				return true
			})
		}
	}

	waitGroup.Wait()
	os.Exit(0)
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

func createListeners(addresses []*net.TCPAddr) (lns []net.Listener, err error) {
	var ln net.Listener
	defer func() {
		if err != nil {
			for _, ln = range lns {
				_ = ln.Close()
			}
		}
	}()
	for _, tcpAddr := range addresses {
		if ln, err = net.ListenTCP("tcp", tcpAddr); err != nil {
			return nil, err
		}
		lns = append(lns, ln)
	}
	return
}

func parseTCPAddress(arr []string) (addresses []*net.TCPAddr, err error) {
	var (
		tcpAddr *net.TCPAddr
	)
	for _, addr := range arr {
		if tcpAddr, err = net.ResolveTCPAddr("tcp", addr); err != nil {
			return nil, err
		}
		addresses = append(addresses, tcpAddr)
	}
	return
}

func listenersDiff(a, b []net.Listener) []net.Listener {
	var (
		diff         []net.Listener
		addrA, addrB *net.TCPAddr
	)
	for _, v := range a {
		var match bool
		addrA = v.Addr().(*net.TCPAddr)
		for _, val := range b {
			addrB = val.Addr().(*net.TCPAddr)
			if addrA.IP.Equal(addrB.IP) && addrA.Port == addrB.Port {
				match = true
				break
			}
		}
		if !match {
			diff = append(diff, v)
		}
	}
	return diff
}

func addrDiff(a, b []*net.TCPAddr) []*net.TCPAddr {
	var diff []*net.TCPAddr
	for _, v := range a {
		var match bool
		for _, val := range b {
			if v.IP.Equal(val.IP) && v.Port == val.Port {
				match = true
				break
			}
		}
		if !match {
			diff = append(diff, v)
		}
	}
	return diff
}

func runWorker() {
	var sig = make(chan os.Signal)
	signal.Notify(sig, syscall.SIGHUP)
	signal.Notify(sig, syscall.SIGTERM)

	v := <-sig
	if v.(syscall.Signal) == syscall.SIGHUP {
		//请求优雅退出
		closeListeners()
		Wait()
	}

	//广播退出事件
	for _, f := range events {
		f()
	}
	os.Exit(0)
}

func createWorker(lns []net.Listener) (*exec.Cmd, error) {
	var (
		err   error
		file  *os.File
		files []*os.File
	)
	for _, v := range lns {
		if file, err = v.(*net.TCPListener).File(); err != nil {
			return nil, err
		}
		files = append(files, file)
	}

	cmd := exec.Command(os.Args[0], os.Args[1:]...)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	cmd.ExtraFiles = files
	return cmd, nil
}
