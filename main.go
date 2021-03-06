package main

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/crypto/secp256k1"
)

// Signature for a function which returns >0 if a>b, <0 if a<b, and 0 otherwise
type addressComparer func(a common.Address, b common.Address) int

type Result struct {
	address    common.Address
	privateKey *ecdsa.PrivateKey
	nonce      int
	depth      int
}

func leastScorer(a, b common.Address) int {
	return -bytes.Compare(a.Bytes(), b.Bytes())
}

func asciiScorer(a, b common.Address) int {
	return countHexrangeDigits(a.Bytes(), false) - countHexrangeDigits(b.Bytes(), false)
}

func ascendingScorer(a, b common.Address) int {
	return countAscending(a.Bytes(), false) - countAscending(b.Bytes(), false)
}

func strictAscendingScorer(a, b common.Address) int {
	return countAscending(a.Bytes(), true) - countAscending(b.Bytes(), true)
}

func countHexrangeDigits(data []byte, strict bool) int {
	count := 0
	for i := 0; i < 20; i++ {
		v := data[i]
		if v >= 32 && v <= 127 {
			count += 1
		}
	}
	return count
}
func toAscii(addr common.Address) string {

	return string(addr.Bytes())
}
func countAscending(data []byte, strict bool) int {
	count := 0
	var last byte = 0
	for i := 0; i < 20; i++ {
		for j := 4; j >= 0; j -= 4 {
			nybble := (data[i] >> uint(j)) & 0xf
			if nybble < last || (nybble > last+1 && strict) {
				return count
			}
			last = nybble
			count += 1
		}
	}
	return 40 // as if
}

type StringList []string

func (sl StringList) String() string {
	return strings.Join([]string(sl), ",")
}

func (sl StringList) Set(value string) error {
	copy(sl, strings.Split(value, ","))
	return nil
}

var (
	threads         = flag.Int("threads", 2, "Number of threads to run")
	contractAddress = flag.Bool("contract", false, "Derive addresses for deployed contracts instead of accounts")
	maxNonce        = flag.Int("maxnonce", 32, "Maximum nonce value to test when deriving contract addresses")
	//scorers         = StringList{"least", "ascending", "strictAscending"}
	scorers    = StringList{"asciiScorer"}
	scoreFuncs = map[string]addressComparer{
		//		"least":           leastScorer,
		//		"ascending":       ascendingScorer,
		//		"strictAscending": strictAscendingScorer,
		"asciiScorer": asciiScorer,
	}
)

func scoreTest(funcs map[string]addressComparer, bests map[string]common.Address, a common.Address) (better bool) {
	for name, scoreFunc := range funcs {
		best, ok := bests[name]
		if !ok || scoreFunc(a, best) >= 0 {
			better = true
			bests[name] = a
		}
	}
	return better
}

func main() {
	flag.Var(scorers, "scorers", "List of score functions to use")
	flag.Parse()

	funcs := make(map[string]addressComparer)
	for _, k := range scorers {
		funcs[k] = scoreFuncs[k]
	}

	results := make(chan Result)
	for i := 0; i < *threads; i++ {
		go start(results, *contractAddress, *maxNonce, funcs)
	}

	bests := make(map[string]common.Address)
	for next := range results {
		if scoreTest(funcs, bests, next.address) {
			if *contractAddress {
				fmt.Printf("%s\t%q\t%d\t%d\t%s\n", next.address.Hex(), toAscii(next.address), next.nonce, next.depth, hex.EncodeToString(crypto.FromECDSA(next.privateKey)))
			} else {
				fmt.Printf("%s\t%d\t%s\n", next.address.Hex(), next.nonce, hex.EncodeToString(crypto.FromECDSA(next.privateKey)))
			}
		}
	}
}

func start(results chan<- Result, contracts bool, maxNonce int, funcs map[string]addressComparer) {
	addresses := make(chan Result)
	go generateAddresses(addresses, contracts, maxNonce, 32)

	bests := make(map[string]common.Address)
	for next := range addresses {
		if scoreTest(funcs, bests, next.address) {
			results <- next
		}
	}
}

func generateAddresses(out chan<- Result, contracts bool, maxNonce int, maxDepth int) {
	for {
		privateKey, err := ecdsa.GenerateKey(secp256k1.S256(), rand.Reader)
		if err != nil {
			fmt.Printf("Error generating ECDSA keypair: %v\n", err)
			os.Exit(1)
		}

		contractAddress := crypto.PubkeyToAddress(privateKey.PublicKey)
		if contracts {
			for i := 0; i < maxNonce; i++ {
				address := crypto.CreateAddress(contractAddress, uint64(i))
				out <- Result{address, privateKey, i, 0}
				//				for j := 1; j < maxDepth; j++ {
				//					address = crypto.CreateAddress(address, 1)
				//					out <- Result{address, privateKey, i, j}
				//				}
			}
		} else {
			out <- Result{contractAddress, privateKey, 0, 0}
		}
	}
	os.Exit(0)
}
