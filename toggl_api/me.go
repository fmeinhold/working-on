package toggl_api

import (
	"encoding/json"
	"time"
)

const (
	path = "me"
)

type MeClient struct {
	config *RequestConfig
	url    string
}

type Me struct {
	Id                 int       `json:"id"`
	ApiToken           string    `json:"api_token"`
	Email              string    `json:"name"`
	FullName           string    `json:"fullname"`
	Timezone           string    `json:"timezone"`
	DefaultWorkspaceId int       `json:"default_workspace_id"`
	BeginningOfWeek    int       `json:"beginning_of_week"`
	ImageURL           string    `json:"image_url"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
	OpenIDEmail        string    `json:"openid_email"`
	OpenIDEnabled      bool      `json:"openid_enabled"`
	CountryID          int       `json:"country_id"`
	HasPassword        bool      `json:"has_password"`
	At                 time.Time `json:"at"`
	IntercomHash       string    `json:"intercom_hash"`
	OauthProviders     []string  `json:"oauth_providers"`
}

func NewMeClient(config *RequestConfig) *MeClient {
	return &MeClient{config: config, url: config.baseURL + "me"}
}

func (c *MeClient) Get() (*Me, error) {
	rawMessage, err := c.config.SendRequest("GET", "me", nil)
	if err != nil {
		return nil, err
	}

	me := &Me{}

	err = json.Unmarshal(*rawMessage, me)
	if err != nil {
		return nil, err
	}

	return me, nil
}
