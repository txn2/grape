package grape

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/patrickmn/go-cache"
	"github.com/pkg/errors"
	"github.com/txn2/ack"
	"github.com/txn2/provision"
	"go.uber.org/zap"
)

var (
	indexRxp       = regexp.MustCompile(`"index".*?:`)
	indexSingleRxp = regexp.MustCompile(`"index".*?:.*?"`)
	indexArrayRxp  = regexp.MustCompile(`"index".*?:.*?\[`)
	accountRxp     = regexp.MustCompile(`^[a-z0-9].*?\-`)
	mappingRxp     = regexp.MustCompile(`^\/\S+\/_mapping$`)
	accountPathRxp = regexp.MustCompile(`^\/.*?-`)
)

// Cfg
type Cfg struct {
	HttpClient       *http.Client
	Logger           *zap.Logger
	PathPrefix       string
	ProvisionService string
}

// Auth
type Auth struct {
	Cfg
	cache *cache.Cache
}

// NewAuth
func NewAuth(cfg Cfg) *Auth {
	cfg.Logger.Info("New request authentication API",
		zap.String("path_prefix", cfg.PathPrefix),
		zap.String("provision_service", cfg.ProvisionService),
	)

	c := cache.New(1*time.Minute, 10*time.Minute)

	return &Auth{cfg, c}
}

// IndexSingle
type IndexSingle struct {
	Index string `json:"index"`
}

// IndexArray
type IndexArray struct {
	Indexes []string `json:"index"`
}

// RequestHandler is reverse proxy middleware for checking calls made
// by the Grafana Elasticsearch datasource. The data source makes two
// type of calls. A GET call for index/_mapping and /_msearch calls for
// batching queries.
//
// Each call is checked for an account prefixed index ACCOUNT-index and
// permission are verified using the name and password segments from BasicAuth
// as key name and key from the corresponding account.
// See: https://godoc.org/github.com/txn2/provision#AccessKey
//
// Elasticsearch datasource documentation see:
// https://grafana.com/docs/features/datasources/elasticsearch/
func (a *Auth) RequestHandler(c *gin.Context) {

	// Authentication
	// get and cache a list of accounts ass
	name, key, ok := c.Request.BasicAuth()
	if !ok {
		ak := ack.Gin(c)
		ak.GinErrorAbort(401, "Unauthorized", "Access requires BasicAuth with access key credentials.")
		return
	}

	accessKey := provision.AccessKey{
		Name: name,
		Key:  key,
	}

	// The only GET operations required for the Grafana
	// datasource plugin is /[ACCOUNT]-index/_mapping all other
	// patterns should be ignored.
	if c.Request.Method == http.MethodGet {

		// check the query string for account
		// parse account
		if mappingRxp.MatchString(c.Request.URL.Path) {
			aP := accountPathRxp.FindString(strings.TrimPrefix(c.Request.URL.Path, a.PathPrefix))
			accountId := aP[1 : len(aP)-1]

			a.Logger.Info("Mapping request", zap.String("account", accountId))

			// check for key access to account
			ok, err := a.checkAccount(accountId, accessKey)
			if err != nil {
				ak := ack.Gin(c)
				ak.GinErrorAbort(401, "AccountLookupError", err.Error())
				a.Logger.Info("Mapping request denied",
					zap.String("key_name", accessKey.Name),
					zap.String("account", accountId),
				)
				return
			}

			// access granted
			if ok {
				return
			}
		}

		// not a _mapping GET or unauthorized key
		ak := ack.Gin(c)
		ak.GinErrorAbort(401, "NonMappingGetRequest", c.Request.URL.Path)
		a.Logger.Info("Non-Mapping GET request denied.",
			zap.String("url", c.Request.URL.Path),
		)
		return
	}

	// /_msearch POST
	if c.Request.Method == http.MethodPost {
		tUrl := strings.TrimPrefix(c.Request.URL.Path, a.PathPrefix)
		if tUrl != "/_msearch" {
			ak := ack.Gin(c)
			ak.SetPayload("Only _msearch POST requests accepted.")
			ak.GinErrorAbort(401, "NonMsearchPostRequest", tUrl)
			a.Logger.Info("Only _msearch POST requests accepted.",
				zap.String("url", tUrl),
			)
			return
		}

		b, err := ioutil.ReadAll(c.Request.Body)
		if err != nil {
			ak := ack.Gin(c)
			ak.GinErrorAbort(401, "MsearchPostBodyError", err.Error())
			return
		}

		// Restore the io.ReadCloser to its original state
		c.Request.Body = ioutil.NopCloser(bytes.NewBuffer(b))

		bytesReader := bytes.NewReader(b)
		scanner := bufio.NewScanner(bytesReader)

		// Each line contains its own JSON object
		for scanner.Scan() {
			ln := scanner.Bytes()

			// does the line contain a reference to an index?
			if !indexRxp.Match(ln) {
				continue
			}

			// does the line contain a reference to a single index?
			if indexSingleRxp.Match(ln) {
				idxSingle := &IndexSingle{}
				err := json.Unmarshal(ln, &idxSingle)
				if err != nil {
					ak := ack.Gin(c)
					ak.GinErrorAbort(401, "MsearchPostBodyIdxLineError", err.Error())
					return
				}

				accountId := strings.TrimSuffix(accountRxp.FindString(idxSingle.Index), "-")
				ok, err := a.checkAccount(accountId, accessKey)
				if err != nil {
					ak := ack.Gin(c)
					ak.GinErrorAbort(401, "AccountLookupError", err.Error())
					a.Logger.Info("Mapping request denied",
						zap.String("key_name", accessKey.Name),
						zap.String("account", accountId),
					)
					return
				}

				// access granted
				if ok {
					return
				}

			}

			// does the line contain a reference to a single index?
			if indexArrayRxp.Match(ln) {
				idxArray := &IndexArray{}
				err := json.Unmarshal(ln, &idxArray)
				if err != nil {
					ak := ack.Gin(c)
					ak.GinErrorAbort(401, "MsearchPostBodyIdxLineError", err.Error())
					return
				}

				for _, idx := range idxArray.Indexes {
					accountId := strings.TrimSuffix(accountRxp.FindString(idx), "-")
					ok, err := a.checkAccount(accountId, accessKey)
					if err != nil {
						ak := ack.Gin(c)
						ak.GinErrorAbort(401, "AccountLookupError", err.Error())
						a.Logger.Info("Mapping request denied",
							zap.String("key_name", accessKey.Name),
							zap.String("account", accountId),
						)
						return
					}

					// on failure return unauthorized
					if !ok {
						ak := ack.Gin(c)
						ak.GinErrorAbort(401, "UnauthorizedIndex", accountId)
						return
					}
				}

			}
		}

		return
	}

	// Anything other than a post including PUT / DELETE etc..
	// should return false. DO NOT LET OTHER OPERATIONS THROUGH
	ak := ack.Gin(c)
	ak.GinErrorAbort(401, "UnauthorizedRequest", c.Request.URL.Path)
}

// checkAccount
func (a *Auth) checkAccount(accountId string, accessKey provision.AccessKey) (bool, error) {
	cacheKey := accountId + accessKey.Name + accessKey.Key

	// check cache
	cacheResult, found := a.cache.Get(cacheKey)
	if found {
		return cacheResult.(bool), nil
	}

	url := a.ProvisionService + "/keyCheck/" + accountId

	accountKeyJson, err := json.Marshal(accessKey)
	if err != nil {
		a.cache.Set(cacheKey, false, cache.DefaultExpiration)
		return false, err
	}
	req, _ := http.NewRequest("POST", url, bytes.NewReader(accountKeyJson))
	res, err := a.HttpClient.Do(req)
	if err != nil {
		a.cache.Set(cacheKey, false, cache.DefaultExpiration)
		return false, err
	}

	if res.StatusCode == 404 {
		a.cache.Set(cacheKey, false, cache.DefaultExpiration)
		return false, errors.New(accountId + " account not found.")
	}

	if res.StatusCode == 200 {
		a.cache.Set(cacheKey, true, cache.DefaultExpiration)
		return true, nil
	}

	a.cache.Set(cacheKey, false, cache.DefaultExpiration)
	return false, errors.New("got code " + string(res.StatusCode) + " from " + url)
}
