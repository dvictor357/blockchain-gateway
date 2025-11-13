package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/dvictor357/blockchain-gateway/pkg/blockchain"
)

func main() {
	fmt.Println("=== Blockchain Library Example ===")

	// Example 1: Simple client manager with common chains
	fmt.Println("\n1. Simple Client Manager:")
	simpleManager, err := blockchain.NewSimpleClientManager(
		blockchain.WithTimeout(10 * time.Second),
	)
	if err != nil {
		log.Fatal(err)
	}

	ctx := context.Background()

	// Get supported chains
	chains := simpleManager.ListChains()
	fmt.Printf("Supported chains: %v\n", chains)

	// Get Ethereum balance (this will fail if no network connection, but shows the API)
	balance, err := simpleManager.QuickBalance(ctx, "ethereum", "0x742d35Cc6634C0532925a3b844Bc454e4438f44e")
	if err != nil {
		fmt.Printf("Balance error (expected if offline): %v\n", err)
	} else {
		fmt.Printf("Balance: %s %s\n", balance.Balance.String(), balance.Symbol)
	}

	// Example 2: Custom client manager
	fmt.Println("\n2. Custom Client Manager:")
	customManager, err := blockchain.NewClientManagerLibrary(
		blockchain.WithTimeout(15 * time.Second),
	)
	if err != nil {
		log.Fatal(err)
	}

	// Add a custom EVM chain
	customClient, err := blockchain.NewEVMClientLibrary(
		"polygon",
		"https://polygon-bor-rpc.publicnode.com",
		blockchain.WithTimeout(20*time.Second),
	)
	if err != nil {
		log.Fatal(err)
	}

	err = customManager.RegisterClient(customClient)
	if err != nil {
		log.Fatal(err)
	}

	// Execute custom RPC call
	result, err := customManager.Execute(ctx, "polygon", "eth_blockNumber", []interface{}{})
	if err != nil {
		fmt.Printf("RPC error (expected if offline): %v\n", err)
	} else {
		fmt.Printf("Polygon block number: %s\n", string(result))
	}

	// Example 3: Individual client usage
	fmt.Println("\n3. Individual Client Usage:")
	ethClient, err := blockchain.NewEVMClientLibrary(
		"ethereum",
		"https://ethereum.publicnode.com",
		blockchain.WithTimeout(5*time.Second),
	)
	if err != nil {
		log.Fatal(err)
	}

	// Get gas price
	gasPrice, err := ethClient.GetGasPrice(ctx)
	if err != nil {
		fmt.Printf("Gas price error (expected if offline): %v\n", err)
	} else {
		fmt.Printf("Ethereum gas price: %s\n", string(gasPrice))
	}

	// Example 4: Bitcoin client
	fmt.Println("\n4. Bitcoin Client:")
	btcClient, err := blockchain.NewBitcoinClientLibrary(
		"https://btc.getblock.io/mainnet",
		blockchain.WithTimeout(10*time.Second),
	)
	if err != nil {
		log.Fatal(err)
	}

	// Get Bitcoin block count
	blockCount, err := btcClient.Execute(ctx, "getblockcount", []interface{}{})
	if err != nil {
		fmt.Printf("Bitcoin error (expected if offline): %v\n", err)
	} else {
		fmt.Printf("Bitcoin block count: %s\n", string(blockCount))
	}

	fmt.Println("\n=== Example completed ===")
}
