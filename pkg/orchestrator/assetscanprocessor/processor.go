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

package assetscanprocessor

import (
	"context"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/openclarity/vmclarity/api/models"
	"github.com/openclarity/vmclarity/pkg/orchestrator/common"
	"github.com/openclarity/vmclarity/pkg/shared/backendclient"
	"github.com/openclarity/vmclarity/pkg/shared/log"
	"github.com/openclarity/vmclarity/pkg/shared/utils"
)

type AssetScanProcessor struct {
	client           *backendclient.BackendClient
	pollPeriod       time.Duration
	reconcileTimeout time.Duration
}

func New(config Config) *AssetScanProcessor {
	return &AssetScanProcessor{
		client:           config.Backend,
		pollPeriod:       config.PollPeriod,
		reconcileTimeout: config.ReconcileTimeout,
	}
}

// Returns true if AssetScanStatus.State is DONE and there are no Errors.
func statusCompletedWithNoErrors(tss *models.AssetScanState) bool {
	return tss != nil && tss.State != nil && *tss.State == models.AssetScanStateStateDone && (tss.Errors == nil || len(*tss.Errors) == 0)
}

// nolint:cyclop
func (asp *AssetScanProcessor) Reconcile(ctx context.Context, event AssetScanReconcileEvent) error {
	// Get latest information, in case we've been sat in the reconcile
	// queue for a while
	assetScan, err := asp.client.GetAssetScan(ctx, event.AssetScanID, models.GetAssetScansAssetScanIDParams{})
	if err != nil {
		return fmt.Errorf("failed to get asset scan from API: %w", err)
	}

	// Re-check the findingsProcessed boolean, we might have been re-queued
	// while already being reconciled, if so we can short circuit here.
	if assetScan.FindingsProcessed != nil && *assetScan.FindingsProcessed {
		return nil
	}

	newFailedToReconcileTypeError := func(err error, t string) error {
		return fmt.Errorf("failed to reconcile asset scan %s %s to findings: %w", *assetScan.Id, t, err)
	}

	// Process each of the successfully scanned (state DONE and no errors) families into findings.
	if statusCompletedWithNoErrors(assetScan.Status.Vulnerabilities) {
		if err := asp.reconcileResultVulnerabilitiesToFindings(ctx, assetScan); err != nil {
			return newFailedToReconcileTypeError(err, "vulnerabilities")
		}
	}

	if statusCompletedWithNoErrors(assetScan.Status.Sbom) {
		if err := asp.reconcileResultPackagesToFindings(ctx, assetScan); err != nil {
			return newFailedToReconcileTypeError(err, "sbom")
		}
	}

	if statusCompletedWithNoErrors(assetScan.Status.Exploits) {
		if err := asp.reconcileResultExploitsToFindings(ctx, assetScan); err != nil {
			return newFailedToReconcileTypeError(err, "exploits")
		}
	}

	if statusCompletedWithNoErrors(assetScan.Status.Secrets) {
		if err := asp.reconcileResultSecretsToFindings(ctx, assetScan); err != nil {
			return newFailedToReconcileTypeError(err, "secrets")
		}
	}

	if statusCompletedWithNoErrors(assetScan.Status.Malware) {
		if err := asp.reconcileResultMalwareToFindings(ctx, assetScan); err != nil {
			return newFailedToReconcileTypeError(err, "malware")
		}
	}

	if statusCompletedWithNoErrors(assetScan.Status.Rootkits) {
		if err := asp.reconcileResultRootkitsToFindings(ctx, assetScan); err != nil {
			return newFailedToReconcileTypeError(err, "rootkits")
		}
	}

	if statusCompletedWithNoErrors(assetScan.Status.Misconfigurations) {
		if err := asp.reconcileResultMisconfigurationsToFindings(ctx, assetScan); err != nil {
			return newFailedToReconcileTypeError(err, "misconfigurations")
		}
	}

	// Mark post-processing completed for this asset scan
	assetScan.FindingsProcessed = utils.PointerTo(true)
	err = asp.client.PatchAssetScan(ctx, assetScan, *assetScan.Id)
	if err != nil {
		return fmt.Errorf("failed to update asset scan %s: %w", *assetScan.Id, err)
	}

	return nil
}

type AssetScanReconcileEvent struct {
	AssetScanID models.AssetScanID
}

func (e AssetScanReconcileEvent) ToFields() logrus.Fields {
	return logrus.Fields{
		"AssetScanID": e.AssetScanID,
	}
}

func (e AssetScanReconcileEvent) String() string {
	return fmt.Sprintf("AssetScanID=%s", e.AssetScanID)
}

func (e AssetScanReconcileEvent) Hash() string {
	return e.AssetScanID
}

func (asp *AssetScanProcessor) GetItems(ctx context.Context) ([]AssetScanReconcileEvent, error) {
	filter := fmt.Sprintf("status/general/state eq '%s' and (findingsProcessed eq false or findingsProcessed eq null)",
		models.AssetScanStateStateDone)
	assetScans, err := asp.client.GetAssetScans(ctx, models.GetAssetScansParams{
		Filter: utils.PointerTo(filter),
		Select: utils.PointerTo("id"),
	})
	if err != nil {
		return []AssetScanReconcileEvent{}, fmt.Errorf("failed to get asset scans from API: %w", err)
	}

	items := make([]AssetScanReconcileEvent, len(*assetScans.Items))
	for i, res := range *assetScans.Items {
		items[i] = AssetScanReconcileEvent{*res.Id}
	}

	return items, nil
}

func (asp *AssetScanProcessor) Start(ctx context.Context) {
	logger := log.GetLoggerFromContextOrDiscard(ctx).WithField("controller", "AssetScanProcessor")
	ctx = log.SetLoggerForContext(ctx, logger)

	queue := common.NewQueue[AssetScanReconcileEvent]()

	poller := common.Poller[AssetScanReconcileEvent]{
		PollPeriod: asp.pollPeriod,
		GetItems:   asp.GetItems,
		Queue:      queue,
	}
	poller.Start(ctx)

	reconciler := common.Reconciler[AssetScanReconcileEvent]{
		ReconcileFunction: asp.Reconcile,
		ReconcileTimeout:  asp.reconcileTimeout,
		Queue:             queue,
	}
	reconciler.Start(ctx)
}
