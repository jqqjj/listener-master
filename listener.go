package master

import (
	"net"
	"sync"
)

type Listener struct {
	net.Listener
	once   sync.Once
	wg     sync.WaitGroup
	worker *worker
}

func newListener(ln net.Listener, worker *worker) *Listener {
	return &Listener{
		Listener: ln,
		worker:   worker,
	}
}

func (l *Listener) Accept() (net.Conn, error) {
	l.wg.Add(1)
	if c, err := l.Listener.Accept(); err != nil {
		l.wg.Done()
		return nil, err
	} else {
		return newConnection(c, l), nil
	}
}

func (l *Listener) Close() error {
	defer func() {
		l.once.Do(func() {
			l.worker.waitGroup.Done()
		})
	}()
	return l.Listener.Close()
}
