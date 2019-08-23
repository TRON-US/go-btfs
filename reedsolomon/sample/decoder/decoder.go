package main

import (
	"flag"
	"fmt"
	"os"

	reedsol "github.com/TRON-US/go-btfs/reedsolomon"

	"github.com/klauspost/reedsolomon"
)

func init() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage of %s:\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  simple-decoder [-flags] basefile.ext\nDo not add the number to the filename.\n")
		fmt.Fprintf(os.Stderr, "Valid flags:\n")
		flag.PrintDefaults()
	}
}

func main() {

	dataShards, parShards, outFile, fname := reedsol.ParseCommandLine()

	//Validate reed-solomon config.
	isValid := reedsol.ValidateConfig(dataShards, parShards)
	if !isValid {
		fmt.Fprintf(os.Stderr, "Error: sum of data and parity shards cannot exceed 256\n")
		os.Exit(1)
	}

	// Create encoding matrix.
	enc, err := reedsolomon.New(*dataShards, *parShards)
	reedsol.CheckErr(err)

	// Create shards and load the data.
	shards := reedsol.CreateShardsFromData(dataShards, parShards, fname)

	// Verify the shards created from files
	reedsol.VerifyShards(shards, enc)

	// Join the shards and write them
	outfn := *outFile
	if outfn == "" {
		outfn = fname
	}

	fmt.Println("Writing data to", outfn)
	f, err := os.Create(outfn)
	reedsol.CheckErr(err)

	// We don't know the exact filesize.
	err = enc.Join(f, shards, len(shards[0])**dataShards)
	reedsol.CheckErr(err)
}
