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
	// Make the current node send hello messages to central at given interval in seconds
	PublisherServiceSayhello int
	// Publisher data folder
	SubscribersDataFolder string
	// central node to receive messages published from nodes
	CentralNodeName string
	// Start the central error logger.
	// Takes a comma separated string of nodes to receive from or "*" for all nodes.
	StartSubErrorLog flagNodeSlice
	// Subscriber for hello messages
	StartSubSayHello flagNodeSlice
	// Subscriber for CLI Commands
	StartSubCLICommand flagNodeSlice
}

func NewConfiguration() *Configuration {
	c := Configuration{}
	return &c
}

// Default configuration
func newConfigurationDefaults() Configuration {
	c := Configuration{
		ConfigFolder:             "./etc",
		BrokerAddress:            "127.0.0.1:4222",
		ProfilingPort:            "",
		PromHostAndPort:          "",
		DefaultMessageTimeout:    10,
		DefaultMessageRetries:    1,
		PublisherServiceSayhello: 30,
		SubscribersDataFolder:    "./data",
		CentralNodeName:          "",
		StartSubErrorLog:         flagNodeSlice{Values: []node{}},
		StartSubSayHello:         flagNodeSlice{Values: []node{}},
		StartSubCLICommand:       flagNodeSlice{Values: []node{}},
	}
	return c
}

func (c *Configuration) CheckFlags() error {

	// Create an empty default config
	var fc Configuration

	// NB: Disabling the config file options for now.
	// Read file config. Set system default if it can't find config file.
	fc, err := c.ReadConfigFile()
	if err != nil {
		log.Printf("%v\n", err)
		fc = newConfigurationDefaults()
	}

	*c = fc

	flag.StringVar(&c.ConfigFolder, "configFolder", fc.ConfigFolder, "folder who contains the config file. Defaults to ./etc/. If other folder is used this flag must be specified at startup.")
	flag.StringVar(&c.NodeName, "nodeName", fc.NodeName, "some unique string to identify this Edge unit")
	flag.StringVar(&c.BrokerAddress, "brokerAddress", fc.BrokerAddress, "the address of the message broker")
	flag.StringVar(&c.ProfilingPort, "profilingPort", fc.ProfilingPort, "The number of the profiling port")
	flag.StringVar(&c.PromHostAndPort, "promHostAndPort", fc.PromHostAndPort, "host and port for prometheus listener, e.g. localhost:2112")
	flag.IntVar(&c.DefaultMessageTimeout, "defaultMessageTimeout", fc.DefaultMessageTimeout, "default message timeout in seconds. This can be overridden on the message level")
	flag.IntVar(&c.DefaultMessageRetries, "defaultMessageRetries", fc.DefaultMessageRetries, "default amount of retries that will be done before a message is thrown away, and out of the system")
	flag.StringVar(&c.SubscribersDataFolder, "subscribersDataFolder", fc.SubscribersDataFolder, "The data folder where subscribers are allowed to write their data if needed")
	flag.StringVar(&c.CentralNodeName, "centralNodeName", fc.CentralNodeName, "The name of the central node to receive messages published by this node")

	flag.IntVar(&c.PublisherServiceSayhello, "publisherServiceSayhello", fc.PublisherServiceSayhello, "Make the current node send hello messages to central at given interval in seconds")

	flag.Var(&c.StartSubErrorLog, "startSubErrorLog", "Specify comma separated list for nodes to allow messages from. Use \"*\" for from all. Value RST will turn off subscriber.")
	flag.Var(&c.StartSubSayHello, "startSubSayHello", "Specify comma separated list for nodes to allow messages from. Use \"*\" for from all. Value RST will turn off subscriber.")
	flag.Var(&c.StartSubCLICommand, "startSubCLICommand", "Specify comma separated list for nodes to allow messages from. Use \"*\" for from all. Value RST will turn off subscriber.")

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
func (c *Configuration) ReadConfigFile() (Configuration, error) {
	fp := filepath.Join("./etc/", "config.toml")

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

	fmt.Printf("%+v\n", c)
	return conf, nil
}

// WriteConfigFile will write the current config to file. If the file or the
// directory for the config file does not exist it will be created.
func (c *Configuration) WriteConfigFile() error {
	if _, err := os.Stat(c.ConfigFolder); os.IsNotExist(err) {
		err := os.Mkdir(c.ConfigFolder, 0700)
		if err != nil {
			return fmt.Errorf("error: failed to create directory %v: %v", c.ConfigFolder, err)
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
