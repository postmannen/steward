package steward

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	toml "github.com/pelletier/go-toml"
)

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
	CentralErrorLogger bool
	// default message timeout in seconds. This can be overridden on the message level")
	DefaultMessageTimeout int
	// default amount of retries that will be done before a message is thrown away, and out of the system
	DefaultMessageRetries int
	// Make the current node send hello messages to central at given interval in seconds
	PublisherServiceSayhello int
}

func NewConfiguration() *Configuration {
	c := Configuration{}
	return &c
}

func (c *Configuration) CheckFlags() {
	// read file config
	fc, err := c.ReadConfigFile()
	if err != nil {
		log.Printf("%v\n", err)
	}

	//fmt.Printf("---\nContent of file\n---\n%#v\n", tmpC)

	flag.StringVar(&c.ConfigFolder, "configFolder", fc.ConfigFolder, "")
	flag.StringVar(&c.NodeName, "node", fc.NodeName, "some unique string to identify this Edge unit")
	flag.StringVar(&c.BrokerAddress, "brokerAddress", fc.BrokerAddress, "the address of the message broker")
	flag.StringVar(&c.ProfilingPort, "profilingPort", fc.ProfilingPort, "The number of the profiling port")
	flag.StringVar(&c.PromHostAndPort, "promHostAndPort", fc.PromHostAndPort, "host and port for prometheus listener, e.g. localhost:2112")
	flag.BoolVar(&c.CentralErrorLogger, "centralErrorLogger", fc.CentralErrorLogger, "set to true if this is the node that should receive the error log's from other nodes")
	flag.IntVar(&c.DefaultMessageTimeout, "defaultMessageTimeout", fc.DefaultMessageTimeout, "default message timeout in seconds. This can be overridden on the message level")
	flag.IntVar(&c.DefaultMessageRetries, "defaultMessageRetries", fc.DefaultMessageRetries, "default amount of retries that will be done before a message is thrown away, and out of the system")
	flag.IntVar(&c.PublisherServiceSayhello, "publisherServiceSayhello", fc.PublisherServiceSayhello, "Make the current node send hello messages to central at given interval in seconds")
	flag.Parse()

	if err := c.WriteConfigFile(); err != nil {
		log.Printf("error: checkFlags: failed writing config file: %v\n", err)
		os.Exit(1)
	}
}

func (c *Configuration) ReadConfigFile() (Configuration, error) {
	fp := filepath.Join("./etc/", "config.toml")

	if _, err := os.Stat(fp); os.IsNotExist(err) {
		return Configuration{}, fmt.Errorf("error: no config file found %v: %v", fp, err)
	}

	f, err := os.OpenFile(fp, os.O_RDONLY, 0600)
	if err != nil {
		return Configuration{}, fmt.Errorf("error: failed to open file %v", err)
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

func (c *Configuration) WriteConfigFile() error {
	if _, err := os.Stat(c.ConfigFolder); os.IsNotExist(err) {
		err := os.Mkdir(c.ConfigFolder, 0600)
		if err != nil {
			return fmt.Errorf("error: failed to create directory %v: %v", c.ConfigFolder, err)
		}
	}

	fp := filepath.Join(c.ConfigFolder, "config.toml")

	f, err := os.OpenFile(fp, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("error: failed to open file %v", err)
	}
	defer f.Close()

	enc := toml.NewEncoder(f)
	enc.Encode(c)

	return nil
}
