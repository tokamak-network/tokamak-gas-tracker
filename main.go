package main

import (
	"context"
	"encoding/csv"
	"fmt"
	"log"
	"math/big"
	"os"
	"strconv"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/params"
)

type Result struct {
	Cost                 *big.Float // ETH
	AvgCallDataGasPrice  *big.Float // Gwei
	AvgBlobGasPrice      *big.Float // Gwei
	TotalCalldataGasUsed uint64
	TotalBlobGasUsed     uint64
	TotalGasUsed         uint64
	TxCount              uint64
}

func main() {
	l1RPC := os.Getenv("L1_RPC")
	fileName := os.Getenv("FILE_NAME")

	client, err := ethclient.Dial(l1RPC)
	if err != nil {
		log.Fatal(err)
	}

	file, err := os.Open(fileName)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	reader := csv.NewReader(file)

	headers, err := reader.Read()
	if err != nil {
		log.Fatal(err)
	}

	var dateTimeIndex, txHashIndex int
	for i, header := range headers {
		if header == "DateTime (UTC)" {
			dateTimeIndex = i
		} else if header == "Transaction Hash" {
			txHashIndex = i
		}
	}

	results := make(map[string]*Result)

	records, err := reader.ReadAll()
	if err != nil {
		log.Fatal(err)
	}

	for _, record := range records {
		dateTime, err := time.Parse("2006-01-02 15:04:05", record[dateTimeIndex])
		if err != nil {
			log.Fatal(err)
		}
		date := dateTime.Format("2006-01-02")

		if results[date] == nil {
			results[date] = &Result{
				Cost:                new(big.Float).SetFloat64(0),
				AvgCallDataGasPrice: new(big.Float).SetUint64(0),
				AvgBlobGasPrice:     new(big.Float).SetUint64(0),
			}
		}

		txHash := common.HexToHash(record[txHashIndex])

		receipt, err := client.TransactionReceipt(context.Background(), txHash)
		if err != nil {
			log.Fatal(err)
		}
		results[date].TxCount += 1

		costWei := calcCost(receipt)
		costEth := weiToEther(costWei)

		results[date].Cost.Add(results[date].Cost, costEth)

		callDataGasPrice := receipt.EffectiveGasPrice
		results[date].AvgCallDataGasPrice.Add(
			results[date].AvgCallDataGasPrice,
			weiToGwei(callDataGasPrice),
		)

		results[date].TotalCalldataGasUsed += receipt.GasUsed

		if receipt.Type == types.BlobTxType {
			blobGasPrice := receipt.BlobGasPrice
			results[date].AvgBlobGasPrice.Add(
				results[date].AvgBlobGasPrice,
				weiToGwei(blobGasPrice),
			)
			results[date].TotalBlobGasUsed += receipt.BlobGasUsed
		}

	}

	for k, v := range results {
		v.AvgCallDataGasPrice.Quo(v.AvgCallDataGasPrice, new(big.Float).SetUint64(v.TxCount))
		v.AvgBlobGasPrice.Quo(v.AvgBlobGasPrice, new(big.Float).SetUint64(v.TxCount))
		v.TotalGasUsed = v.TotalCalldataGasUsed + v.TotalBlobGasUsed

		fmt.Printf("%s: %v\n", k, v)
	}

	outFile, err := os.Create(fmt.Sprintf("./outputs/output-%s", fileName))
	if err != nil {
		log.Fatal(err)
	}
	defer outFile.Close()

	writer := csv.NewWriter(outFile)
	defer writer.Flush()

	header := []string{
		"DateTime",
		"Total Cost(ETH)",
		"Avg Calldata gas price(Gwei)",
		"Avg Blob Gas Price(Gwei)",
		"Total Calldata Gas Used",
		"Total Blob Gas Used",
		"Total Gas Used(calldata + blob)",
		"Transaction Count",
	}
	if err := writer.Write(header); err != nil {
		log.Fatal(err)
	}
	for k, v := range results {
		record := []string{
			k,
			v.Cost.String(),
			v.AvgCallDataGasPrice.String(),
			v.AvgBlobGasPrice.String(),
			strconv.FormatUint(v.TotalCalldataGasUsed, 10),
			strconv.FormatUint(v.TotalBlobGasUsed, 10),
			strconv.FormatUint(v.TotalGasUsed, 10),
			strconv.FormatUint(v.TxCount, 10),
		}
		if err := writer.Write(record); err != nil {
			log.Fatal(err)
		}
	}
}

func weiToEther(wei *big.Int) *big.Float {
	return new(big.Float).Quo(new(big.Float).SetInt(wei), big.NewFloat(params.Ether))
}

func weiToGwei(wei *big.Int) *big.Float {
	return new(big.Float).Quo(new(big.Float).SetInt(wei), big.NewFloat(params.GWei))
}

func calcCost(r *types.Receipt) *big.Int {
	total := new(big.Int).Mul(r.EffectiveGasPrice, new(big.Int).SetUint64(r.GasUsed))
	if r.Type == types.BlobTxType {
		total.Add(
			total,
			new(big.Int).Mul(r.BlobGasPrice, new(big.Int).SetUint64(r.BlobGasUsed)),
		)
	}
	return total
}
