// Package conf parses start up args and config file
package conf

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/MG-RAST/golib/goconfig/config"
	"gopkg.in/yaml.v2"
)

// Location set of storage locations
type LocationConfig struct {
	ID          string `bson:"ID" json:"ID" yaml:"ID" `                           // e.g. ANLs3 or local for local store
	Description string `bson:"Description" json:"Description" yaml:"Description"` // e.g. ANL official S3 service
	Type        string `bson:"type" json:"type" yaml:"Type" `                     // e.g. S3
	URL         string `bson:"url" json:"url" yaml:"URL"`                         // e.g. http://s3api.invalid.org/download&id=
	Token       string `bson:"token" json:"-" yaml:"Token" `                      // e.g.  Key or password
	Prefix      string `bson:"prefix" json:"-" yaml:"Prefix"`                     // e.g. any prefix needed
	AuthKey     string `bson:"AuthKey" json:"-" yaml:"AuthKey"`                   // e.g. AWS auth-key
	Persistent  bool   `bson:"persistent" json:"persistent" yaml:"Persistent"`    // e.g. is this a valid long term storage location
	Priority    int    `bson:"priority" json:"priority" yaml:"Priority"`          // e.g. location priority for pushing files upstream to this location, 0 is lowest, 100 highest
	MinPriority int    `bson:"minpriority" json:"minpriority" yaml:"MinPriority"` // e.g. minimum node priority level for this location (e.g. some stores will only handle non temporary files or high value files)
	Tier        int    `bson:"tier" json:"tier" yaml:"Tier"`                      // e.g. class or tier 0= cache, 3=ssd based backend, 5=disk based backend, 10=tape archive
	Cost        int    `bson:"cost" json:"cost" yaml:"Cost"`                      // e.g. dollar cost per GB for this store, default=0

	S3Location  `bson:",inline" json:",inline" yaml:",inline"` // extensions specific to S3
	TSMLocation `bson:",inline" json:",inline" yaml:",inline"` // extension sspecific to IBM TSM
}

// S3Location S3 specific fields
type S3Location struct {
	Bucket    string `bson:"bucket" json:"bucket" yaml:"Bucket" `
	Region    string `bson:"region" json:"region" yaml:"Region" `
	SecretKey string `bson:"SecretKey" json:"-" yaml:"SecretKey" ` // e.g.g AWS secret-key
}

// TSMLocation IBM TSM specific fields
type TSMLocation struct {
	Recoverycommand string `bson:"recoverycommand" json:"recoverycommand" yaml:"Recoverycommand" `
}

// LocationsMap allow access to Location objects via Locations("ID")
var LocationsMap map[string]*LocationConfig

// Config contains an array of Location objects
type Config struct {
	Locations []LocationConfig `bson:"Locations" json:"Locations" yaml:"Locations" `
}

type idxOpts struct {
	unique   bool
	dropDups bool
	sparse   bool
}

//const VERSION string = "[% VERSION %]"
var VERSION string

var VERSIONS = map[string]int{
	"ACL":  2,
	"Auth": 1,
	"Node": 4,
}

var LOG_OUTPUTS = [3]string{"file", "console", "both"}

var (
	// Admin
	ADMIN_EMAIL string
	ADMIN_USERS string
	AdminUsers  []string

	// Permissions for anonymous user
	ANON_READ   bool
	ANON_WRITE  bool
	ANON_DELETE bool

	// Address
	API_IP   string
	API_PORT int
	API_URL  string // for external address only

	// Auth
	AUTH_BASIC              bool
	AUTH_GLOBUS_TOKEN_URL   string
	AUTH_GLOBUS_PROFILE_URL string
	AUTH_OAUTH_URL_STR      string
	AUTH_OAUTH_BEARER_STR   string
	AUTH_CACHE_TIMEOUT      int
	AUTH_OAUTH              = make(map[string]string)
	OAUTH_DEFAULT           string // first value in AUTH_OAUTH_URL_STR

	// Default Chunksize for size virtual index
	CHUNK_SIZE int64 = 1048576

	// Config File
	CONFIG_FILE string

	// Runtime
	EXPIRE_WAIT   int // wait time for reaper in minutes
	GOMAXPROCS    string
	MAX_REVISIONS int // max number of node revisions to keep; values < 0 mean keep all

	// Logs
	LOG_PERF    bool // Indicates whether performance logs should be stored
	LOG_ROTATE  bool // Indicates whether logs should be rotated daily
	LOG_OUTPUT  string
	LOG_TRACE   bool // enable trace logging
	DEBUG_LEVEL int

	// Mongo information
	MONGODB_HOSTS             string
	MONGODB_DATABASE          string
	MONGODB_USER              string
	MONGODB_PASSWORD          string
	MONGODB_ATTRIBUTE_INDEXES string

	// Node Indices
	NODE_IDXS map[string]idxOpts = nil

	// Paths
	PATH_SITE    string
	PATH_DATA    string
	PATH_LOGS    string
	PATH_LOCAL   string
	PATH_PIDFILE string

	// Reload
	RELOAD string

	// SSL
	SSL      bool
	SSL_KEY  string
	SSL_CERT string

	PRINT_HELP   bool // full usage
	SHOW_HELP    bool // simple usage
	SHOW_VERSION bool

	// change behavior of system from cache to backend store
	//IS_CACHE   bool   //
	PATH_CACHE        string //   path to cache directory, default is PATH_DATA
	MIN_REPLICA_COUNT int    // minimum number of Locations required before enabling delete of local file in DATA_PATH
	CACHE_TTL         int    // time in hours for cache items to be retained

	// internal config control
	FAKE_VAR = false
)

// Initialize is an explicit init. Enables outside use
// of shock-server packages. Parses config and populates
// the conf variables.
func Initialize() (err error) {

	for i, elem := range os.Args {
		if strings.HasPrefix(elem, "-conf") || strings.HasPrefix(elem, "--conf") {
			parts := strings.SplitN(elem, "=", 2)
			if len(parts) == 2 {
				CONFIG_FILE = parts[1]
			} else if i+1 < len(os.Args) {
				CONFIG_FILE = os.Args[i+1]
			} else {
				return errors.New("parsing command options, missing conf file")
			}
		}
	}

	var c *config.Config = nil
	if CONFIG_FILE != "" {
		c, err = config.ReadDefault(CONFIG_FILE)
		if err != nil {
			return errors.New("error reading conf file: " + err.Error())
		}
		fmt.Printf("read %s\n", CONFIG_FILE)
	} else {
		fmt.Printf("No config file specified.\n")
		c = config.NewDefault()
	}

	c_store, err := getConfiguration(c) // from config file and command line arguments
	if err != nil {
		return errors.New("error reading conf file: " + err.Error())
	}

	// ####### at this point configuration variables are set ########

	err = parseConfiguration()
	if err != nil {
		return errors.New("error parsing conf file: " + err.Error())
	}

	if FAKE_VAR == false {
		return errors.New("config was not parsed")
	}
	if PRINT_HELP || SHOW_HELP {
		c_store.PrintHelp()
		os.Exit(0)
	}
	if SHOW_VERSION {
		fmt.Printf("Shock version: %s\n", VERSION)
		os.Exit(0)
	}

	// read Locations.yaml file from same directory as config file
	var LocationsPath = path.Dir(CONFIG_FILE)
	LocationsPath = path.Join(LocationsPath, "Locations.yaml")

	fmt.Printf("read Locations file: %s\n", LocationsPath)

	// we should check the YAML config file for correctness and schema compliance
	// TOBEADDED --> https://github.com/santhosh-tekuri/jsonschema/issues/5
	err = readYAMLConfig(LocationsPath)
	if err != nil {
		return errors.New("error reading Locations file: " + err.Error())
	}
	return
}

// Bool is a convenience wrapper around strconv.ParseBool
func Bool(s string) bool {
	b, _ := strconv.ParseBool(s)
	return b
}

// Print prints the configuration loads to stdout
func Print() {
	fmt.Printf("####### Anonymous ######\nread:\t%v\nwrite:\t%v\ndelete:\t%v\n\n", ANON_READ, ANON_WRITE, ANON_DELETE)
	if (AUTH_GLOBUS_TOKEN_URL != "" && AUTH_GLOBUS_PROFILE_URL != "") || len(AUTH_OAUTH) > 0 {
		fmt.Printf("##### Auth #####\n")
		if AUTH_GLOBUS_TOKEN_URL != "" && AUTH_GLOBUS_PROFILE_URL != "" {
			fmt.Printf("type:\tglobus\ntoken_url:\t%s\nprofile_url:\t%s\n\n", AUTH_GLOBUS_TOKEN_URL, AUTH_GLOBUS_PROFILE_URL)
		}
		if len(AUTH_OAUTH) > 0 {
			fmt.Printf("type:\toauth\n")
			for b, u := range AUTH_OAUTH {
				fmt.Printf("bearer: %s\turl: %s\n", b, u)
			}
			fmt.Printf("\n")
		}
	}
	fmt.Printf("##### Admin #####\nusers:\t%s\n\n", ADMIN_USERS)
	fmt.Printf("##### Paths #####\nsite:\t%s\ndata:\t%s\nlogs:\t%s\nlocal_paths:\t%s\ncache_path:\t%s\n\n", PATH_SITE, PATH_DATA, PATH_LOGS, PATH_LOCAL, PATH_CACHE)
	if SSL {
		fmt.Printf("##### SSL enabled #####\n")
		fmt.Printf("##### SSL key:\t%s\n##### SSL cert:\t%s\n\n", SSL_KEY, SSL_CERT)
	} else {
		fmt.Printf("##### SSL disabled #####\n\n")
	}
	fmt.Printf("##### Mongodb #####\nhost(s):\t%s\ndatabase:\t%s\nattribute_indexes:\t%s\n\n", MONGODB_HOSTS, MONGODB_DATABASE, MONGODB_ATTRIBUTE_INDEXES)
	fmt.Printf("##### Address #####\nip:\t%s\nport:\t%d\n\n", API_IP, API_PORT)
	if LOG_PERF {
		fmt.Printf("##### PerfLog enabled #####\n\n")
	}
	if LOG_ROTATE {
		fmt.Printf("##### Log rotation enabled #####\n\n")
	} else {
		fmt.Printf("##### Log rotation disabled #####\n\n")
	}
	fmt.Printf("##### Expiration #####\nexpire_wait:\t%d minutes\n\n", EXPIRE_WAIT)
	fmt.Printf("##### Minimum Replica Count in other Locations #####\nmin_replica_count:\t%d (before removing local files)\n\n", MIN_REPLICA_COUNT)
	fmt.Printf("##### Max Revisions #####\nmax_revisions:\t%d\n\n", MAX_REVISIONS)
	fmt.Printf("API_PORT: %d\n", API_PORT)
}

func getConfiguration(c *config.Config) (c_store *Config_store, err error) {
	c_store = NewCS(c)

	// Admin
	c_store.AddString(&ADMIN_EMAIL, "", "Admin", "email", "", "")
	c_store.AddString(&ADMIN_USERS, "", "Admin", "users", "", "")

	// Access-Control
	c_store.AddBool(&ANON_READ, true, "Anonymous", "read", "", "")
	c_store.AddBool(&ANON_WRITE, true, "Anonymous", "write", "", "")
	c_store.AddBool(&ANON_DELETE, true, "Anonymous", "delete", "", "")

	// Address
	c_store.AddString(&API_IP, "0.0.0.0", "Address", "api-ip", "", "")
	c_store.AddInt(&API_PORT, 7445, "Address", "api-port", "", "")

	// URLs
	c_store.AddString(&API_URL, "http://localhost", "External", "api-url", "", "")

	// Auth
	c_store.AddBool(&AUTH_BASIC, false, "Auth", "basic", "", "")
	c_store.AddString(&AUTH_GLOBUS_TOKEN_URL, "", "Auth", "globus_token_url", "", "")
	c_store.AddString(&AUTH_GLOBUS_PROFILE_URL, "", "Auth", "globus_profile_url", "", "")
	c_store.AddString(&AUTH_OAUTH_URL_STR, "", "Auth", "oauth_urls", "", "")
	c_store.AddString(&AUTH_OAUTH_BEARER_STR, "", "Auth", "oauth_bearers", "", "")
	c_store.AddInt(&AUTH_CACHE_TIMEOUT, 60, "Auth", "cache_timeout", "", "")

	// Runtime
	c_store.AddInt(&EXPIRE_WAIT, 60, "Runtime", "expire_wait", "", "")
	c_store.AddString(&GOMAXPROCS, "", "Runtime", "GOMAXPROCS", "", "")
	c_store.AddInt(&MAX_REVISIONS, 3, "Runtime", "max_revisions", "", "")

	// Log
	c_store.AddBool(&LOG_PERF, false, "Log", "perf_log", "", "")
	c_store.AddBool(&LOG_ROTATE, true, "Log", "rotate", "", "")
	c_store.AddString(&LOG_OUTPUT, "both", "Log", "logoutput", "console, file or both", "")
	c_store.AddBool(&LOG_TRACE, false, "Log", "trace", "", "")
	c_store.AddInt(&DEBUG_LEVEL, 0, "Log", "debuglevel", "debug level: 0-3", "")

	// Mongodb
	c_store.AddString(&MONGODB_ATTRIBUTE_INDEXES, "", "Mongodb", "attribute_indexes", "", "")
	c_store.AddString(&MONGODB_DATABASE, "ShockDB", "Mongodb", "database", "", "")
	c_store.AddString(&MONGODB_HOSTS, "mongo", "Mongodb", "hosts", "", "")
	c_store.AddString(&MONGODB_PASSWORD, "", "Mongodb", "password", "", "")
	c_store.AddString(&MONGODB_USER, "", "Mongodb", "user", "", "")

	// parse Node-Indices
	NODE_IDXS = map[string]idxOpts{}
	nodeIdx, _ := c.Options("Node-Indices")
	for _, opt := range nodeIdx {
		val, _ := c.String("Node-Indices", opt)
		opts := idxOpts{}
		for _, parts := range strings.Split(val, ",") {
			p := strings.Split(parts, ":")
			if p[0] == "unique" {
				if p[1] == "true" {
					opts.unique = true
				} else {
					opts.unique = false
				}
			} else if p[0] == "dropDups" {
				if p[1] == "true" {
					opts.dropDups = true
				} else {
					opts.dropDups = false
				}
			} else if p[0] == "sparse" {
				if p[1] == "true" {
					opts.sparse = true
				} else {
					opts.sparse = false
				}
			}
		}
		NODE_IDXS[opt] = opts
	}

	// Paths
	c_store.AddString(&PATH_SITE, "/usr/local/shock/site", "Paths", "site", "", "")
	c_store.AddString(&PATH_DATA, "/usr/local/shock/data", "Paths", "data", "", "")
	c_store.AddString(&PATH_LOGS, "/var/log/shock", "Paths", "logs", "", "")
	c_store.AddString(&PATH_LOCAL, "/var/tmp", "Paths", "local_paths", "", "")
	c_store.AddString(&PATH_PIDFILE, "", "Paths", "pidfile", "", "")

	// cache
	c_store.AddString(&PATH_CACHE, "", "Paths", "cache_path", "", "cache directory path, default is nil, if this is set the system will function as a cache")
	c_store.AddInt(&CACHE_TTL, 24.0, "Cache", "cache_ttl", "", "ttl in hours for cache items")
	c_store.AddInt(&MIN_REPLICA_COUNT, 2, "Other", "min_replica_count", "", "minimum number of locations required before enabling local Node file deletion")

	// SSL
	c_store.AddBool(&SSL, false, "SSL", "enable", "", "")
	c_store.AddString(&SSL_KEY, "", "SSL", "key", "", "")
	c_store.AddString(&SSL_CERT, "", "SSL", "cert", "", "")

	// Other - thses option are CLI only
	c_store.AddString(&RELOAD, "", "Other", "reload", "path or url to shock data. WARNING this will drop all current data.", "")
	gopath := os.Getenv("GOPATH")
	c_store.AddString(&CONFIG_FILE, gopath+"/src/github.com/MG-RAST/Shock/shock-server.conf.template", "Other", "conf", "path to config file", "")
	c_store.AddBool(&SHOW_VERSION, false, "Other", "version", "show version", "")
	c_store.AddBool(&PRINT_HELP, false, "Other", "fullhelp", "show detailed usage without \"--\"-prefixes", "")
	c_store.AddBool(&SHOW_HELP, false, "Other", "help", "show usage", "")

	c_store.Parse()
	return
}

func parseConfiguration() (err error) {
	// get admin users
	if ADMIN_USERS != "" {
		for _, name := range strings.Split(ADMIN_USERS, ",") {
			AdminUsers = append(AdminUsers, strings.TrimSpace(name))
		}
	}

	// parse OAuth settings if used
	if AUTH_OAUTH_URL_STR != "" && AUTH_OAUTH_BEARER_STR != "" {
		ou := strings.Split(AUTH_OAUTH_URL_STR, ",")
		ob := strings.Split(AUTH_OAUTH_BEARER_STR, ",")
		if len(ou) != len(ob) {
			return errors.New("number of items in oauth_urls and oauth_bearers are not the same")
		}
		for i := range ob {
			AUTH_OAUTH[ob[i]] = ou[i]
		}
		OAUTH_DEFAULT = ou[0] // first url is default for "oauth" bearer token
	}

	// validate LOG_OUTPUT
	vaildLogout := false
	for _, logout := range LOG_OUTPUTS {
		if LOG_OUTPUT == logout {
			vaildLogout = true
		}
	}
	if !vaildLogout {
		return errors.New("invalid option for logoutput, use one of: file, console, both")
	}

	// clean paths
	PATH_SITE = cleanPath(PATH_SITE)
	PATH_DATA = cleanPath(PATH_DATA)
	PATH_LOGS = cleanPath(PATH_LOGS)
	PATH_LOCAL = cleanPath(PATH_LOCAL)
	PATH_PIDFILE = cleanPath(PATH_PIDFILE)
	PATH_CACHE = cleanPath(PATH_CACHE)

	return
}

func cleanPath(p string) string {
	if p != "" {
		p, _ = filepath.Abs(p)
	}
	return p
}

// readYAMLConfig read a YAML style config file with Shock configuration
// the file has to be a yaml file, currently for Locations only
func readYAMLConfig(filename string) (err error) {

	var conf Config

	source, err := ioutil.ReadFile(filename)
	if err != nil {
		return (err)
	}
	err = yaml.Unmarshal(source, &conf)
	if err != nil {
		return (err)
	}

	// create a global
	//var Locations Locations
	LocationsMap = make(map[string]*LocationConfig)

	for i, _ := range conf.Locations {
		loc := &conf.Locations[i]

		LocationsMap[loc.ID] = loc
	}

	return
}
