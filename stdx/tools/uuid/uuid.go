package main

import (
	"fmt"
	"math"
	"os"

	"markdown.ninja/stdx/cobra"
	"markdown.ninja/stdx/uuid"
)

const (
	defautlnumberOfUuidsToGenerateToGenerate = 1
	version                                  = "1.0.0"
)

var (
	flagNumber  uint64
	flagVersion uint8
)

func init() {
	rootCmd.Flags().Uint64VarP(&flagNumber, "number", "n", defautlnumberOfUuidsToGenerateToGenerate, "Number of UUIDs to generate")
	rootCmd.Flags().Uint8VarP(&flagVersion, "version", "v", 4, "UUID version (valid values: [4, 7])")
}

func main() {
	err := rootCmd.Execute()
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}
}

var rootCmd = &cobra.Command{
	Use:   "uuid",
	Short: "Generate UUIDs. Version: " + version,
	// Version:       version,
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) (err error) {
		if flagNumber > math.MaxInt32 {
			err = fmt.Errorf("Can't generate more than %d UUIDs", math.MaxInt32)
			return
		}

		if flagVersion != 4 && flagVersion != 7 {
			err = fmt.Errorf("Invalid UUID version: %d. Valid values are: [4, 7]", flagVersion)
			return
		}

		for range flagNumber {
			var id uuid.UUID

			switch flagVersion {
			case 4:
				id = uuid.NewV4()
			case 7:
				id = uuid.NewV7()
			}
			fmt.Println(id.String())
		}

		return
	},
}
