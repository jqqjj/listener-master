package master

import (
	"net"
	"sync"
)

type Connection struct {
	net.Conn
	wg   *sync.WaitGroup
	once *sync.Once
}

func newConnection(wg *sync.WaitGroup, conn net.Conn) *Connection {
	return &Connection{Conn: conn, wg: wg, once: new(sync.Once)}
}

func (c *Connection) Close() error {
	c.once.Do(func() {
		c.wg.Done()
	})
	return c.Conn.Close()
}
