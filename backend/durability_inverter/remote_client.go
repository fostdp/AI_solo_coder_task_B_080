package durability_inverter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"aqueduct-monitor/models"
)

type RemoteInversionClient struct {
	baseURL    string
	httpClient *http.Client
}

type RemoteInversionRequest struct {
	SegmentID          string  `json:"segment_id"`
	ObservedWeathering float64 `json:"observed_weathering_cm"`
	ObservedStrength   float64 `json:"observed_strength_mpa"`
	ObservedPH         float64 `json:"observed_ph"`
	AgeYears           float64 `json:"age_years"`
}

type RemoteInversionResponse struct {
	Success       bool                              `json:"success"`
	Message       string                            `json:"message,omitempty"`
	Result        *InversionResult                  `json:"result,omitempty"`
	Confidence    *ConfidenceMetrics                `json:"confidence,omitempty"`
	BestFormula   *models.RomanConcreteFormula      `json:"best_formula,omitempty"`
	Candidates    []models.InversionFormulaCandidate `json:"candidates,omitempty"`
	ProcessTimeMs int64                             `json:"process_time_ms"`
}

type RemoteBatchRequest struct {
	Requests []RemoteInversionRequest `json:"requests"`
}

type RemoteBatchResponse struct {
	Success     bool                    `json:"success"`
	Results     []RemoteInversionResponse `json:"results"`
	TotalTimeMs int64                   `json:"total_time_ms"`
}

func NewRemoteInversionClient(baseURL string) *RemoteInversionClient {
	return &RemoteInversionClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (c *RemoteInversionClient) Invert(
	ctx context.Context,
	observedWeathering float64,
	observedStrength float64,
	observedPH float64,
	ageYears float64,
	segmentID string,
) (*RemoteInversionResponse, error) {

	req := RemoteInversionRequest{
		SegmentID:          segmentID,
		ObservedWeathering: observedWeathering,
		ObservedStrength:   observedStrength,
		ObservedPH:         observedPH,
		AgeYears:           ageYears,
	}

	reqBody, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST",
		c.baseURL+"/api/v1/invert", bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}

	var result RemoteInversionResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	if !result.Success {
		return nil, fmt.Errorf("inversion failed: %s", result.Message)
	}

	return &result, nil
}

func (c *RemoteInversionClient) BatchInvert(
	ctx context.Context,
	requests []RemoteInversionRequest,
) (*RemoteBatchResponse, error) {

	batchReq := RemoteBatchRequest{Requests: requests}
	reqBody, err := json.Marshal(batchReq)
	if err != nil {
		return nil, fmt.Errorf("marshal batch request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST",
		c.baseURL+"/api/v1/batch-invert", bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, fmt.Errorf("create batch request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("send batch request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}

	var result RemoteBatchResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode batch response: %w", err)
	}

	if !result.Success {
		return nil, fmt.Errorf("batch inversion failed")
	}

	return &result, nil
}

func (c *RemoteInversionClient) GetFormulas(ctx context.Context) ([]models.RomanConcreteFormula, error) {
	httpReq, err := http.NewRequestWithContext(ctx, "GET",
		c.baseURL+"/api/v1/formulas", nil)
	if err != nil {
		return nil, fmt.Errorf("create formulas request: %w", err)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("send formulas request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Count    int                           `json:"count"`
		Formulas []models.RomanConcreteFormula `json:"formulas"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode formulas response: %w", err)
	}

	return result.Formulas, nil
}

func (c *RemoteInversionClient) HealthCheck(ctx context.Context) (bool, error) {
	httpReq, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/health", nil)
	if err != nil {
		return false, err
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK, nil
}
