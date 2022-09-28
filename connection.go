package master

import (
	"net"
	"sync"
)

type Connection struct {
	net.Conn
	once     sync.Once
	listener *Listener
}

func newConnection(conn net.Conn, ln *Listener) *Connection {
	return &Connection{Conn: conn, listener: ln}
}

func (c *Connection) Close() error {
	defer func() {
		c.once.Do(func() {
			c.listener.wg.Done()
		})
	}()
	return c.Conn.Close()
}
