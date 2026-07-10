package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type CoreClient struct {
	MasterURL string
	BotSecret string
	HTTP      *http.Client
}

type UserUpsertRequest struct {
	TgID    int64  `json:"tg_id"`
	Tier    string `json:"tier"`
	AddDays int64  `json:"add_days,omitempty"`
}

type UserResponse struct {
	TgID          int64   `json:"tg_id"`
	Tier          string  `json:"tier"`
	ExpireAt      *string `json:"expire_at"`
	VlessUUID     *string `json:"vless_uuid"`
	Hy2Password   *string `json:"hy2_password"`
	TuicUUID      *string `json:"tuic_uuid"`
	TuicPassword  *string `json:"tuic_password"`
}

func NewCoreClient(url, secret string) *CoreClient {
	return &CoreClient{
		MasterURL: url,
		BotSecret: secret,
		HTTP:      &http.Client{Timeout: 5 * time.Second},
	}
}

func (c *CoreClient) sendPost(path string, body any) (int, error) {
	data, _ := json.Marshal(body)
	req, err := http.NewRequest("POST", c.MasterURL+path, bytes.NewBuffer(data))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.BotSecret)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	return resp.StatusCode, nil
}

func (c *CoreClient) CreateUser(tgID int64, tier string) error {
	status, err := c.sendPost("/api/v1/users", UserUpsertRequest{TgID: tgID, Tier: tier})
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return fmt.Errorf("master returned status: %d", status)
	}
	return nil
}

func (c *CoreClient) GetUser(tgID int64) (*UserResponse, error) {
	req, err := http.NewRequest("GET", fmt.Sprintf("%s/api/v1/users/%d", c.MasterURL, tgID), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.BotSecret)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("master returned status: %d", resp.StatusCode)
	}

	var user UserResponse
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return nil, err
	}
	return &user, nil
}

func (c *CoreClient) ApprovePayment(paymentID int) error {
	status, err := c.sendPost(fmt.Sprintf("/api/v1/payments/%d/approve", paymentID), nil)
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return fmt.Errorf("master returned status: %d", status)
	}
	return nil
}

func (c *CoreClient) RejectPayment(paymentID int) error {
	status, err := c.sendPost(fmt.Sprintf("/api/v1/payments/%d/reject", paymentID), nil)
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return fmt.Errorf("master returned status: %d", status)
	}
	return nil
}

func (c *CoreClient) DeleteUser(tgID int64) error {
	req, err := http.NewRequest("DELETE", fmt.Sprintf("%s/api/v1/users/%d", c.MasterURL, tgID), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.BotSecret)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("user not found")
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("master returned status: %d", resp.StatusCode)
	}
	return nil
}
