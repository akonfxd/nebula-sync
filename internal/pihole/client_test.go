package pihole

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	tc "github.com/testcontainers/testcontainers-go"

	"github.com/lovelaze/nebula-sync/e2e"
	"github.com/lovelaze/nebula-sync/internal/pihole/model"
)

const (
	apiPassword string = "test"
)

var (
	piHole     = e2e.RunPiHole(apiPassword).Container
	httpClient = http.DefaultClient
)

type clientTestSuite struct {
	suite.Suite
	client Client
}

func (suite *clientTestSuite) SetupTest() {
	client := createClient(piHole)
	err := client.PostAuth(suite.T().Context())
	suite.Require().NoError(err)
	suite.client = client
}

func TestClientIntegration(t *testing.T) {
	suite.Run(t, new(clientTestSuite))
}

func (suite *clientTestSuite) TestClient_Authenticate() {
	err := suite.client.PostAuth(suite.T().Context())

	suite.Require().NoError(err)
}

func (suite *clientTestSuite) TestClient_DeleteSession() {
	err := suite.client.DeleteSession(suite.T().Context())

	suite.Require().NoError(err)
}

func (suite *clientTestSuite) TestClient_GetTeleporter() {
	payload, err := suite.client.GetTeleporter(suite.T().Context())

	suite.Require().NoError(err)
	suite.NotNil(suite.T(), payload)
}

func (suite *clientTestSuite) TestClient_PostTeleporter() {
	payload, _ := suite.client.GetTeleporter(suite.T().Context())
	err := suite.client.PostTeleporter(suite.T().Context(), payload, &model.PostTeleporterRequest{
		Config:     true,
		DHCPLeases: true,
		Gravity: model.PostGravityRequest{
			Group:             true,
			Adlist:            true,
			AdlistByGroup:     true,
			Domainlist:        true,
			DomainlistByGroup: true,
			Client:            true,
			ClientByGroup:     true,
		},
	})

	suite.Require().NoError(err)
}

func (suite *clientTestSuite) TestClient_GetConfig() {
	conf, err := suite.client.GetConfig(suite.T().Context())

	suite.Require().NoError(err)
	suite.NotNil(suite.T(), conf)
}

func (suite *clientTestSuite) TestClient_PatchConfig() {
	request := model.PatchConfigRequest{
		Config: model.PatchConfig{
			DNS:      nil,
			DHCP:     nil,
			NTP:      nil,
			Resolver: nil,
			Database: nil,
			Misc:     nil,
			Debug:    nil,
		},
	}
	err := suite.client.PatchConfig(suite.T().Context(), &request)

	suite.Require().NoError(err)
}

func (suite *clientTestSuite) TestClient_PostRunGravity() {
	err := suite.client.PostRunGravity(suite.T().Context())

	suite.Require().NoError(err)
}

func TestClient_String(t *testing.T) {
	piHole := model.NewPiHole("http://asdfasdf.com:1234", apiPassword)
	s := NewClient(piHole, httpClient).String()

	assert.Equal(t, "http://asdfasdf.com:1234", s)
}

func TestClient_ApiPath(t *testing.T) {
	piHole := model.NewPiHole("http://asdfasdf.com:1234", apiPassword)
	c := NewClient(piHole, httpClient)

	url := c.String()
	path := c.APIPath("testing")
	expectedPath := fmt.Sprintf("%s/api/testing", url)

	assert.Equal(t, expectedPath, path)
}

func Test_auth_verify(t *testing.T) {
	a := auth{
		sid:      "",
		csrf:     "",
		validity: 0,
		valid:    false,
	}
	require.Error(t, a.verify())

	a.valid = true
	require.NoError(t, a.verify())
}

func createClient(container tc.Container) Client {
	apiPort, err := container.MappedPort(context.Background(), "80/tcp")
	if err != nil {
		panic(err)
	}

	host := fmt.Sprintf("http://localhost:%s", apiPort.Port())

	return NewClient(model.NewPiHole(host, apiPassword), httpClient)
}

// Building a client is what initService and Reload do on every configuration
// change, including the one where there is no primary yet.
func TestNewClient_toleratesAnUnconfiguredInstance(t *testing.T) {
	client := NewClient(model.PiHole{}, http.DefaultClient)

	assert.Empty(t, client.String())
	assert.Empty(t, client.APIPath("config"))
	assert.Error(t, client.PostAuth(context.Background()))
}
