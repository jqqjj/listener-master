package master

import (
	"net"
	"sync"
)

var (
	listeners  []*Listener
	wgListener sync.WaitGroup
)

type Listener struct {
	net.Listener
	wg   *sync.WaitGroup
	once *sync.Once
}

func parseListeners(lns []net.Listener) []*Listener {
	wgListener.Add(len(lns))
	for _, ln := range lns {
		listeners = append(listeners, &Listener{
			Listener: ln,
			wg:       new(sync.WaitGroup),
			once:     new(sync.Once),
		})
	}
	return listeners
}

func (l *Listener) Accept() (net.Conn, error) {
	l.wg.Add(1)
	if c, err := l.Listener.Accept(); err != nil {
		l.wg.Done()
		return nil, err
	} else {
		return newConnection(l.wg, c), nil
	}
}

func (l *Listener) Close() error {
	l.once.Do(func() {
		wgListener.Done()
	})
	return l.Listener.Close()
}

func Wait() {
	wgListener.Wait()
	for _, v := range listeners {
		v.wg.Wait()
	}
}

func closeListeners() {
	for _, ln := range listeners {
		ln.Close()
	}
}
