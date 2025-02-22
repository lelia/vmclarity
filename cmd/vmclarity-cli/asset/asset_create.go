// Copyright © 2023 Cisco Systems, Inc. and its affiliates.
// All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package asset

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/openclarity/vmclarity/cmd/vmclarity-cli/logutil"
	cliutils "github.com/openclarity/vmclarity/pkg/cli/utils"

	"github.com/openclarity/vmclarity/api/models"
	"github.com/openclarity/vmclarity/pkg/shared/backendclient"
)

// AssetCreateCmd represents the standalone command.
var AssetCreateCmd = &cobra.Command{
	Use:   "asset-create",
	Short: "Create asset",
	Long:  `It creates asset. It's useful in the CI/CD mode without VMClarity orchestration`,
	Run: func(cmd *cobra.Command, args []string) {
		logutil.Logger.Infof("Creating asset...")
		filename, err := cmd.Flags().GetString("file")
		if err != nil {
			logutil.Logger.Fatalf("Unable to get asset json file name: %v", err)
		}
		server, err := cmd.Flags().GetString("server")
		if err != nil {
			logutil.Logger.Fatalf("Unable to get VMClarity server address: %v", err)
		}
		assetType, err := getAssetFromJSONFile(filename)
		if err != nil {
			logutil.Logger.Fatalf("Failed to get asset from json file: %v", err)
		}
		updateIfExists, err := cmd.Flags().GetBool("update-if-exists")
		if err != nil {
			logutil.Logger.Fatalf("Unable to get update-if-exists flag vaule: %v", err)
		}
		jsonPath, err := cmd.Flags().GetString("jsonpath")
		if err != nil {
			logutil.Logger.Fatalf("Unable to get jsonpath: %v", err)
		}

		_, err = assetType.ValueByDiscriminator()
		if err != nil {
			logutil.Logger.Fatalf("Failed to determine asset type: %v", err)
		}

		asset, err := createAsset(context.TODO(), assetType, server, updateIfExists)
		if err != nil {
			logutil.Logger.Fatalf("Failed to create asset: %v", err)
		}

		if err := cliutils.PrintJSONData(asset, jsonPath); err != nil {
			logutil.Logger.Fatalf("Failed to print jsonpath: %v", err)
		}
	},
}

func init() {
	AssetCreateCmd.Flags().String("file", "", "asset json filename")
	AssetCreateCmd.Flags().String("server", "", "VMClarity server to create asset to, for example: http://localhost:9999/api")
	AssetCreateCmd.Flags().Bool("update-if-exists", false, "the asset will be updated the asset if it exists")
	AssetCreateCmd.Flags().String("jsonpath", "", "print selected value of asset")
	if err := AssetCreateCmd.MarkFlagRequired("file"); err != nil {
		logutil.Logger.Fatalf("Failed to mark file flag as required: %v", err)
	}
	if err := AssetCreateCmd.MarkFlagRequired("server"); err != nil {
		logutil.Logger.Fatalf("Failed to mark server flag as required: %v", err)
	}
}

func getAssetFromJSONFile(filename string) (*models.AssetType, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %v", err)
	}
	defer file.Close()

	// get the file size
	stat, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("failed to get file stat: %v", err)
	}

	// read the file
	bs := make([]byte, stat.Size())
	_, err = file.Read(bs)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %v", err)
	}

	assetType := &models.AssetType{}
	if err := assetType.UnmarshalJSON(bs); err != nil {
		return nil, fmt.Errorf("failed to unmarshal asset into AssetType %v", err)
	}

	return assetType, nil
}

func createAsset(ctx context.Context, assetType *models.AssetType, server string, updateIfExists bool) (*models.Asset, error) {
	client, err := backendclient.Create(server)
	if err != nil {
		return nil, fmt.Errorf("failed to create VMClarity API client: %w", err)
	}

	creationTime := time.Now()
	assetData := models.Asset{
		AssetInfo: assetType,
		LastSeen:  &creationTime,
		FirstSeen: &creationTime,
	}
	asset, err := client.PostAsset(ctx, assetData)
	if err == nil {
		return asset, nil
	}
	var conflictError backendclient.AssetConflictError
	// As we got a conflict it means there is an existing asset
	// which matches the unique properties of this asset, in this
	// case if the update-if-exists flag is set we'll patch the just AssetInfo and FirstSeen instead.
	if !errors.As(err, &conflictError) || !updateIfExists {
		return nil, fmt.Errorf("failed to post asset: %v", err)
	}
	assetData.FirstSeen = nil
	err = client.PatchAsset(ctx, assetData, *conflictError.ConflictingAsset.Id)
	if err != nil {
		return nil, fmt.Errorf("failed to patch asset: %v", err)
	}

	return conflictError.ConflictingAsset, nil
}
