package main

import (
	"fmt"
	"net"

	binrpc "github.com/florentchauveau/go-kamailio-binrpc"
)

func main() {
	var conn net.Conn

	conn, err := net.Dial("tcp", "localhost:2049")

	if err != nil {
		panic(err)
	}

	cookie, err := binrpc.WritePacketString(conn, "tm.stats")

	if err != nil {
		panic(err)
	}

	records, err := binrpc.ReadPacket(conn, cookie)

	if err != nil {
		panic(err)
	}

	fmt.Printf("records = %v", records)
}
