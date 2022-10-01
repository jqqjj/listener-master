package master

import (
	"net"
	"os"
	"sync"
)

type Listener struct {
	net.Listener
	file   *os.File
	once   sync.Once
	wg     sync.WaitGroup
	worker *worker
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
			if l.worker != nil {
				l.worker.waitGroup.Done()
			}
		})
	}()
	if l.file != nil {
		_ = l.file.Close()
	}
	return l.Listener.Close()
}
