package main

import "github.com/canopy-network/go-plugin/contract"

func main() {
	contract.StartPlugin(contract.DefaultConfig())
	select {} // block forever
}
