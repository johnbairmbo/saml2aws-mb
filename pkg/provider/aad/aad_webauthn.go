package aad

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/versent/saml2aws/v2/pkg/creds"
	"github.com/versent/saml2aws/v2/pkg/provider/okta"
)

type AzureADFidoResponse struct {
	Challenge     string `json:"challenge"`
	RPId          string `json:"rpId"`
	AllowedCreds  []struct {
		ID   string `json:"id"`
		Type string `json:"type"`
	} `json:"allowCredentials"`
	Timeout int `json:"timeout"`
}

func (ac *Client) processFidoAuthentication(convergedResponse *ConvergedResponse, loginDetails *creds.LoginDetails) (*http.Response, error) {
	if !convergedResponse.FIsFidoSupported || convergedResponse.URLFidoLogin == "" {
		return nil, errors.New("FIDO2 authentication not supported or configured")
	}

	req, err := http.NewRequest("GET", convergedResponse.URLFidoLogin, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating FIDO2 challenge request: %w", err)
	}

	res, err := ac.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error retrieving FIDO2 challenge: %w", err)
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading FIDO2 challenge response: %w", err)
	}

	var fidoResponse AzureADFidoResponse
	if err := json.Unmarshal(body, &fidoResponse); err != nil {
		return nil, fmt.Errorf("error parsing FIDO2 challenge: %w", err)
	}

	var credentialID string
	if len(fidoResponse.AllowedCreds) > 0 {
		credentialID = fidoResponse.AllowedCreds[0].ID
	}

	fidoClient, err := okta.NewFidoClient(
		fidoResponse.Challenge,
		fidoResponse.RPId,
		"",
		credentialID,
		convergedResponse.SFT,
		new(okta.U2FDeviceFinder),
	)
	if err != nil {
		return nil, fmt.Errorf("error creating FIDO client: %w", err)
	}

	signedAssertion, err := fidoClient.ChallengeU2F()
	if err != nil {
		return nil, fmt.Errorf("error during FIDO2 authentication: %w", err)
	}

	payload, err := json.Marshal(signedAssertion)
	if err != nil {
		return nil, fmt.Errorf("error marshaling FIDO2 response: %w", err)
	}

	submitReq, err := http.NewRequest("POST", convergedResponse.URLPost, strings.NewReader(string(payload)))
	if err != nil {
		return nil, fmt.Errorf("error creating FIDO2 submit request: %w", err)
	}

	submitReq.Header.Add("Content-Type", "application/json")
	return ac.client.Do(submitReq)
}
