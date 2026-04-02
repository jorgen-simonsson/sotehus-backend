package main

import (
	"fmt"
	"os"
	"strconv"

	"github.com/simonvetter/modbus"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintf(os.Stderr, "Usage: %s <device-ip> <modbus-address>\n", os.Args[0])
		os.Exit(1)
	}

	ip := os.Args[1]
	addr, err := strconv.ParseUint(os.Args[2], 10, 16)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Invalid modbus address: %v\n", err)
		os.Exit(1)
	}

	client, err := modbus.NewClient(&modbus.ClientConfiguration{
		URL:     fmt.Sprintf("tcp://%s:1502", ip),
		Timeout: 5 * 1000000000, // 5 seconds
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create modbus client: %v\n", err)
		os.Exit(1)
	}

	err = client.Open()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open connection: %v\n", err)
		os.Exit(1)
	}
	defer client.Close()

	reg, err := client.ReadRegister(uint16(addr), modbus.HOLDING_REGISTER)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to read register %d: %v\n", addr, err)
		os.Exit(1)
	}

	fmt.Printf("Register %d: %d (0x%04X)\n", addr, reg, reg)
}
