package steward

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	toml "github.com/pelletier/go-toml"
)

// --- flag string slice

// flagStringSlice is a type used when a flag value contains
// comma separated values like `-myflag="somevalue,anothervalue`.
// If a flag of this type is used, and it contains a value, the
// Set(string) method will call the Parse() method.
// The comma separated content will then be split, and put into
// the []values field, and the `ok` field will be set to true, so
// it can be used to check if the flag was used and contained any
// values.
type flagNodeSlice struct {
	value  string
	OK     bool
	Values []node
}

// String method
func (f *flagNodeSlice) String() string {
	return ""
}

// Set will be called when this flag type is used as a flag.Var.
// It will put the comma separated string value given as input into
// the `value`field, then call the Parse function to split those
// comma separated values into []values.
func (f *flagNodeSlice) Set(s string) error {
	f.value = s
	f.Parse()
	return nil
}

// If the flag value "RST" is given, set the default values
// for each of the flag var's fields, and return back.
// Since we reset the actual flag values, it is also these
// blank values that will be written to the config file, and
// making the change persistent in the system. This will also
// be reflected in values stored in the config file, since the
// config file is written after the flags have been parsed.
func (f *flagNodeSlice) Parse() error {
	if len(f.value) == 0 {
		return nil
	}

	split := strings.Split(f.value, ",")

	// Reset values if RST was the flag value.
	if split[0] == "RST" {
		f.OK = false
		f.value = ""
		f.Values = []node{}
		return nil
	}

	fv := f.value
	sp := strings.Split(fv, ",")
	f.OK = true
	f.Values = []node{}

	for _, v := range sp {
		f.Values = append(f.Values, node(v))
	}
	return nil
}

// --- Configuration
type Configuration struct {
	// The configuration folder on disk
	ConfigFolder string
	// The folder where the socket file should live
	SocketFolder string
	// The folder where the database should live
	DatabaseFolder string
	// some unique string to identify this Edge unit
	NodeName string
	// the address of the message broker
	BrokerAddress string
	// The number of the profiling port
	ProfilingPort string
	// host and port for prometheus listener, e.g. localhost:2112
	PromHostAndPort string
	// set to true if this is the node that should receive the error log's from other nodes
	DefaultMessageTimeout int
	// default amount of retries that will be done before a message is thrown away, and out of the system
	DefaultMessageRetries int
	// Publisher data folder
	SubscribersDataFolder string
	// central node to receive messages published from nodes
	CentralNodeName string
	// Path to the certificate of the root CA
	RootCAPath string

	// Full path to the NKEY's seed file
	NkeySeedFile string

	// Make the current node send hello messages to central at given interval in seconds
	StartPubREQHello int
	// Start the central error logger.
	// Takes a comma separated string of nodes to receive from or "*" for all nodes.
	StartSubREQErrorLog flagNodeSlice
	// Subscriber for hello messages
	StartSubREQHello flagNodeSlice
	// Subscriber for text logging
	StartSubREQToFileAppend flagNodeSlice
	// Subscriber for writing to file
	StartSubREQToFile flagNodeSlice
	// Subscriber for Echo Request
	StartSubREQPing flagNodeSlice
	// Subscriber for Echo Reply
	StartSubREQPong flagNodeSlice
	// Subscriber for CLICommandRequest
	StartSubREQCliCommand flagNodeSlice
	// Subscriber for REQnCliCommand
	StartSubREQnCliCommand flagNodeSlice
	// Subscriber for REQToConsole
	StartSubREQToConsole flagNodeSlice
	// Subscriber for REQHttpGet
	StartSubREQHttpGet flagNodeSlice
	// Subscriber for tailing log files
	StartSubREQTailFile flagNodeSlice
}

// NewConfiguration will set a default Configuration,
// and return a *Configuration.
func NewConfiguration() *Configuration {
	c := Configuration{}
	return &c
}

// Default configuration
func newConfigurationDefaults() Configuration {
	c := Configuration{
		ConfigFolder:            "/usr/local/steward/etc/",
		SocketFolder:            "./tmp",
		DatabaseFolder:          "./var/lib",
		BrokerAddress:           "127.0.0.1:4222",
		ProfilingPort:           "",
		PromHostAndPort:         "",
		DefaultMessageTimeout:   10,
		DefaultMessageRetries:   1,
		StartPubREQHello:        30,
		SubscribersDataFolder:   "./var",
		CentralNodeName:         "",
		RootCAPath:              "",
		NkeySeedFile:            "",
		StartSubREQErrorLog:     flagNodeSlice{Values: []node{}},
		StartSubREQHello:        flagNodeSlice{OK: true, Values: []node{"*"}},
		StartSubREQToFileAppend: flagNodeSlice{OK: true, Values: []node{"*"}},
		StartSubREQToFile:       flagNodeSlice{OK: true, Values: []node{"*"}},
		StartSubREQPing:         flagNodeSlice{OK: true, Values: []node{"*"}},
		StartSubREQPong:         flagNodeSlice{OK: true, Values: []node{"*"}},
		StartSubREQCliCommand:   flagNodeSlice{OK: true, Values: []node{"*"}},
		StartSubREQnCliCommand:  flagNodeSlice{OK: true, Values: []node{"*"}},
		StartSubREQToConsole:    flagNodeSlice{OK: true, Values: []node{"*"}},
		StartSubREQHttpGet:      flagNodeSlice{OK: true, Values: []node{"*"}},
		StartSubREQTailFile:     flagNodeSlice{OK: true, Values: []node{"*"}},
	}
	return c
}

// CheckFlags will parse all flags
func (c *Configuration) CheckFlags() error {

	// Create an empty default config
	var fc Configuration

	// Read file config. Set system default if it can't find config file.
	configFolder := os.Getenv("CONFIGFOLDER")
	fc, err := c.ReadConfigFile(configFolder)
	if err != nil {
		log.Printf("%v\n", err)
		fc = newConfigurationDefaults()
	}

	*c = fc

	flag.StringVar(&c.ConfigFolder, "configFolder", fc.ConfigFolder, "Defaults to ./usr/local/steward/etc/. *NB* This flag is not used, if your config file are located somwhere else than default set the location in an env variable named CONFIGFOLDER")
	flag.StringVar(&c.SocketFolder, "socketFolder", fc.SocketFolder, "folder who contains the socket file. Defaults to ./tmp/. If other folder is used this flag must be specified at startup.")
	flag.StringVar(&c.DatabaseFolder, "databaseFolder", fc.DatabaseFolder, "folder who contains the database file. Defaults to ./var/lib/. If other folder is used this flag must be specified at startup.")
	flag.StringVar(&c.NodeName, "nodeName", fc.NodeName, "some unique string to identify this Edge unit")
	flag.StringVar(&c.BrokerAddress, "brokerAddress", fc.BrokerAddress, "the address of the message broker")
	flag.StringVar(&c.ProfilingPort, "profilingPort", fc.ProfilingPort, "The number of the profiling port")
	flag.StringVar(&c.PromHostAndPort, "promHostAndPort", fc.PromHostAndPort, "host and port for prometheus listener, e.g. localhost:2112")
	flag.IntVar(&c.DefaultMessageTimeout, "defaultMessageTimeout", fc.DefaultMessageTimeout, "default message timeout in seconds. This can be overridden on the message level")
	flag.IntVar(&c.DefaultMessageRetries, "defaultMessageRetries", fc.DefaultMessageRetries, "default amount of retries that will be done before a message is thrown away, and out of the system")
	flag.StringVar(&c.SubscribersDataFolder, "subscribersDataFolder", fc.SubscribersDataFolder, "The data folder where subscribers are allowed to write their data if needed")
	flag.StringVar(&c.CentralNodeName, "centralNodeName", fc.CentralNodeName, "The name of the central node to receive messages published by this node")
	flag.StringVar(&c.RootCAPath, "rootCAPath", fc.RootCAPath, "If TLS, enter the path for where to find the root CA certificate")
	flag.StringVar(&c.NkeySeedFile, "nkeySeedFile", fc.NkeySeedFile, "The full path of the nkeys seed file")

	flag.IntVar(&c.StartPubREQHello, "startPubREQHello", fc.StartPubREQHello, "Make the current node send hello messages to central at given interval in seconds")

	flag.Var(&c.StartSubREQErrorLog, "startSubREQErrorLog", "Specify comma separated list for nodes to allow messages from. Use \"*\" for from all. Value RST will turn off subscriber.")
	flag.Var(&c.StartSubREQHello, "startSubREQHello", "Specify comma separated list for nodes to allow messages from. Use \"*\" for from all. Value RST will turn off subscriber.")
	flag.Var(&c.StartSubREQToFileAppend, "startSubREQToFileAppend", "Specify comma separated list for nodes to allow messages from. Use \"*\" for from all. Value RST will turn off subscriber.")
	flag.Var(&c.StartSubREQToFile, "startSubREQToFile", "Specify comma separated list for nodes to allow messages from. Use \"*\" for from all. Value RST will turn off subscriber.")
	flag.Var(&c.StartSubREQPing, "startSubREQPing", "Specify comma separated list for nodes to allow messages from. Use \"*\" for from all. Value RST will turn off subscriber.")
	flag.Var(&c.StartSubREQPong, "startSubREQPong", "Specify comma separated list for nodes to allow messages from. Use \"*\" for from all. Value RST will turn off subscriber.")
	flag.Var(&c.StartSubREQCliCommand, "startSubREQCliCommand", "Specify comma separated list for nodes to allow messages from. Use \"*\" for from all. Value RST will turn off subscriber.")
	flag.Var(&c.StartSubREQnCliCommand, "startSubREQnCliCommand", "Specify comma separated list for nodes to allow messages from. Use \"*\" for from all. Value RST will turn off subscriber.")
	flag.Var(&c.StartSubREQToConsole, "startSubREQToConsole", "Specify comma separated list for nodes to allow messages from. Use \"*\" for from all. Value RST will turn off subscriber.")
	flag.Var(&c.StartSubREQHttpGet, "startSubREQHttpGet", "Specify comma separated list for nodes to allow messages from. Use \"*\" for from all. Value RST will turn off subscriber.")
	flag.Var(&c.StartSubREQTailFile, "startSubREQTailFile", "Specify comma separated list for nodes to allow messages from. Use \"*\" for from all. Value RST will turn off subscriber.")

	flag.Parse()

	// Check that mandatory flag values have been set.
	switch {
	case c.NodeName == "":
		return fmt.Errorf("error: the nodeName config option or flag cannot be empty, check -help")
	case c.CentralNodeName == "":
		return fmt.Errorf("error: the centralNodeName config option or flag cannot be empty, check -help")
	}

	// NB: Disabling the config file options for now.
	if err := c.WriteConfigFile(); err != nil {
		log.Printf("error: checkFlags: failed writing config file: %v\n", err)
		os.Exit(1)
	}

	return nil
}

// Reads the current config file from disk.
func (c *Configuration) ReadConfigFile(configFolder string) (Configuration, error) {
	fp := filepath.Join(configFolder, "config.toml")

	if _, err := os.Stat(fp); os.IsNotExist(err) {
		return Configuration{}, fmt.Errorf("error: no config file found %v: %v", fp, err)
	}

	f, err := os.OpenFile(fp, os.O_RDONLY, 0600)
	if err != nil {
		return Configuration{}, fmt.Errorf("error: ReadConfigFile: failed to open file: %v", err)
	}
	defer f.Close()

	var conf Configuration
	dec := toml.NewDecoder(f)
	err = dec.Decode(&conf)
	if err != nil {
		return Configuration{}, fmt.Errorf("error: decode toml file %v: %v", fp, err)
	}

	// fmt.Printf("%+v\n", c)
	return conf, nil
}

// WriteConfigFile will write the current config to file. If the file or the
// directory for the config file does not exist it will be created.
func (c *Configuration) WriteConfigFile() error {
	if _, err := os.Stat(c.ConfigFolder); os.IsNotExist(err) {
		err := os.MkdirAll(c.ConfigFolder, 0700)
		if err != nil {
			return fmt.Errorf("error: failed to create config directory %v: %v", c.ConfigFolder, err)
		}
	}

	fp := filepath.Join(c.ConfigFolder, "config.toml")

	f, err := os.OpenFile(fp, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("error: WriteConfigFile: failed to open file: %v", err)
	}
	defer f.Close()

	enc := toml.NewEncoder(f)
	enc.Encode(c)

	return nil
}
