package master

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

type master struct {
	mux             sync.Mutex
	waitGroup       sync.WaitGroup
	listenerMap     map[net.Listener][]*exec.Cmd
	workers         map[*exec.Cmd]struct{}
	resolveAddrFunc func() []string
}

func newMaster(resolveAddrFunc func() []string) *master {
	return &master{
		listenerMap:     make(map[net.Listener][]*exec.Cmd),
		workers:         make(map[*exec.Cmd]struct{}),
		resolveAddrFunc: resolveAddrFunc,
	}
}

func (m *master) run() {
	var (
		err       error
		addresses []string
		lns       []net.Listener
		cmd       *exec.Cmd
	)

	addresses = m.resolveAddrFunc()
	if len(addresses) == 0 {
		panic("resolveAddrFunc resolve empty addr")
	}
	if lns, err = m.resolveStringAddrListener(addresses...); err != nil {
		panic(err)
	}
	if cmd, err = m.createCmd(lns...); err != nil {
		panic(err)
	}

	m.waitGroup.Add(1)
	m.attach(cmd, lns...)
	go func(cmd *exec.Cmd) {
		defer m.waitGroup.Done()
		defer m.detach(cmd)
		if err = cmd.Start(); err != nil {
			panic(err)
		}
		cmd.Wait()
	}(cmd)

	m.loopEvent()
}

func (m *master) loopEvent() {
	sig := make(chan os.Signal)
	signal.Notify(sig, syscall.SIGHUP)
	signal.Notify(sig, syscall.SIGINT)
	signal.Notify(sig, syscall.SIGTERM)

	for v := range sig {
		if v.(syscall.Signal) == syscall.SIGINT || v.(syscall.Signal) == syscall.SIGTERM {
			go func() {
				for i := 0; i <= 3; i++ {
					m.mux.Lock()
					for cmd := range m.workers {
						if i == 3 {
							_ = cmd.Process.Kill()
						} else {
							_ = cmd.Process.Signal(syscall.SIGTERM)
						}
					}
					m.mux.Unlock()
					time.Sleep(time.Second)
				}
			}()
			break
		}
		if v.(syscall.Signal) == syscall.SIGHUP {
			var (
				err       error
				lns       []net.Listener
				cmd       *exec.Cmd
				addresses = m.resolveAddrFunc()
			)

			if len(addresses) == 0 {
				fmt.Println("address is empty")
				continue
			}
			if lns, err = m.resolveStringAddrListener(addresses...); err != nil {
				fmt.Println(err)
				continue
			}
			if cmd, err = m.createCmd(lns...); err != nil {
				fmt.Println(err)
				continue
			}

			m.waitGroup.Add(1)
			m.attach(cmd, lns...)
			go func(cmd *exec.Cmd) {
				defer m.waitGroup.Done()
				defer m.detach(cmd)
				if err = cmd.Start(); err != nil {
					panic(err)
				}
				cmd.Wait()
			}(cmd)

			//请求优雅退出
			m.mux.Lock()
			for worker := range m.workers {
				if cmd != worker {
					_ = worker.Process.Signal(syscall.SIGHUP)
				}
			}
			m.mux.Unlock()
		}
	}

	m.waitGroup.Wait()
	os.Exit(0)
}

func (m *master) attach(cmd *exec.Cmd, lns ...net.Listener) {
	m.mux.Lock()
	defer m.mux.Unlock()

	for _, ln := range lns {
		if _, ok := m.listenerMap[ln]; !ok {
			m.listenerMap[ln] = make([]*exec.Cmd, 0)
		}
		match := false
		for _, item := range m.listenerMap[ln] {
			if item == cmd {
				match = true
				break
			}
		}
		if !match {
			m.listenerMap[ln] = append(m.listenerMap[ln], cmd)
		}
	}
	m.workers[cmd] = struct{}{}
}

func (m *master) detach(cmd *exec.Cmd) {
	m.mux.Lock()
	defer m.mux.Unlock()

	for ln, commands := range m.listenerMap {
		index := -1
		for i, item := range commands {
			if item == cmd {
				index = i
				break
			}
		}
		if index >= 0 {
			m.listenerMap[ln] = append(m.listenerMap[ln][:index], m.listenerMap[ln][index+1:]...)
		}
		if len(m.listenerMap[ln]) == 0 {
			_ = ln.Close()
			delete(m.listenerMap, ln)
		}
	}
	for _, file := range cmd.ExtraFiles {
		_ = file.Close()
	}
	delete(m.workers, cmd)
}

func (m *master) resolveStringAddrListener(address ...string) ([]net.Listener, error) {
	var (
		err     error
		tcpAddr []*net.TCPAddr
	)
	if tcpAddr, err = m.resolveAddr(address...); err != nil {
		return nil, err
	}
	return m.resolveTcpAddrListener(tcpAddr...)
}

func (m *master) resolveTcpAddrListener(addresses ...*net.TCPAddr) ([]net.Listener, error) {
	var (
		err    error
		ln     net.Listener
		lns    []net.Listener
		newLns []net.Listener
	)
	defer func() {
		if err != nil {
			for _, l := range newLns {
				_ = l.Close()
			}
		}
	}()
	for _, addr := range addresses {
		if ln = m.findListener(addr); ln == nil {
			if ln, err = net.ListenTCP("tcp", addr); err != nil {
				return nil, err
			}
			newLns = append(newLns, ln)
		}
		lns = append(lns, ln)
	}
	return lns, nil
}

func (m *master) resolveAddr(address ...string) ([]*net.TCPAddr, error) {
	var (
		err      error
		iterator *net.TCPAddr
		tcpAddr  []*net.TCPAddr
	)
	for _, v := range address {
		if iterator, err = net.ResolveTCPAddr("tcp", v); err != nil {
			return nil, err
		}
		tcpAddr = append(tcpAddr, iterator)
	}
	return tcpAddr, nil
}

func (m *master) findListener(addr *net.TCPAddr) net.Listener {
	m.mux.Lock()
	defer m.mux.Unlock()

	for ln := range m.listenerMap {
		lnAddr := ln.Addr().(*net.TCPAddr)
		if lnAddr.Port == addr.Port {
			if lnAddr.IP.IsUnspecified() && (addr.IP == nil || addr.IP.IsUnspecified()) {
				return ln
			}
			if lnAddr.IP.Equal(addr.IP) {
				return ln
			}
		}
	}
	return nil
}

func (m *master) createCmd(lns ...net.Listener) (*exec.Cmd, error) {
	var (
		err   error
		file  *os.File
		files []*os.File
	)
	defer func() {
		if err != nil {
			for _, file = range files {
				_ = file.Close()
			}
		}
	}()

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
