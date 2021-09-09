package steward

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	toml "github.com/pelletier/go-toml"
)

// Configuration are the structure that holds all the different
// configuration options used both with flags and the config file.
// If a new field is added to this struct there should also be
// added the same field to the ConfigurationFromFile struct, and
// an if check should be added to the checkConfigValues function
// to set default values when reading from config file.
type Configuration struct {
	// The configuration folder on disk
	ConfigFolder string
	// The folder where the socket file should live
	SocketFolder string
	// TCP Listener for sending messages to the system
	TCPListener string
	// The folder where the database should live
	DatabaseFolder string
	// some unique string to identify this Edge unit
	NodeName string
	// the address of the message broker
	BrokerAddress string
	// nats connect retry
	NatsConnectRetryInterval int
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
	// The host and port to expose the data folder
	ExposeDataFolder string
	// Timeout for error messages
	ErrorMessageTimeout int
	// Retries for error messages.
	ErrorMessageRetries int
	// Make the current node send hello messages to central at given interval in seconds
	StartPubREQHello int
	// Start the central error logger.
	// Takes a comma separated string of nodes to receive from or "*" for all nodes.
	StartSubREQErrorLog bool
	// Subscriber for hello messages
	StartSubREQHello bool
	// Subscriber for text logging
	StartSubREQToFileAppend bool
	// Subscriber for writing to file
	StartSubREQToFile bool
	// Subscriber for Echo Request
	StartSubREQPing bool
	// Subscriber for Echo Reply
	StartSubREQPong bool
	// Subscriber for CLICommandRequest
	StartSubREQCliCommand bool
	// Subscriber for REQnCliCommand
	StartSubREQnCliCommand bool
	// Subscriber for REQToConsole
	StartSubREQToConsole bool
	// Subscriber for REQHttpGet
	StartSubREQHttpGet bool
	// Subscriber for tailing log files
	StartSubREQTailFile bool
	// Subscriber for continously delivery of output from cli commands.
	StartSubREQnCliCommandCont bool
}

// ConfigurationFromFile should have the same structure as
// Configuration. This structure is used when parsing the
// configuration values from file, so we are able to detect
// if a value were given or not when parsing.
type ConfigurationFromFile struct {
	ConfigFolder               *string
	SocketFolder               *string
	TCPListener                *string
	DatabaseFolder             *string
	NodeName                   *string
	BrokerAddress              *string
	NatsConnectRetryInterval   *int
	ProfilingPort              *string
	PromHostAndPort            *string
	DefaultMessageTimeout      *int
	DefaultMessageRetries      *int
	SubscribersDataFolder      *string
	CentralNodeName            *string
	RootCAPath                 *string
	NkeySeedFile               *string
	ExposeDataFolder           *string
	ErrorMessageTimeout        *int
	ErrorMessageRetries        *int
	StartPubREQHello           *int
	StartSubREQErrorLog        *bool
	StartSubREQHello           *bool
	StartSubREQToFileAppend    *bool
	StartSubREQToFile          *bool
	StartSubREQPing            *bool
	StartSubREQPong            *bool
	StartSubREQCliCommand      *bool
	StartSubREQnCliCommand     *bool
	StartSubREQToConsole       *bool
	StartSubREQHttpGet         *bool
	StartSubREQTailFile        *bool
	StartSubREQnCliCommandCont *bool
}

// NewConfiguration will return a *Configuration.
func NewConfiguration() *Configuration {
	c := Configuration{}
	return &c
}

// Get a Configuration struct with the default values set.
func newConfigurationDefaults() Configuration {
	c := Configuration{
		ConfigFolder:             "./etc/",
		SocketFolder:             "./tmp",
		TCPListener:              "",
		DatabaseFolder:           "./var/lib",
		BrokerAddress:            "127.0.0.1:4222",
		NatsConnectRetryInterval: 10,
		ProfilingPort:            "",
		PromHostAndPort:          "",
		DefaultMessageTimeout:    10,
		DefaultMessageRetries:    1,
		StartPubREQHello:         30,
		SubscribersDataFolder:    "./data",
		CentralNodeName:          "",
		RootCAPath:               "",
		NkeySeedFile:             "",
		ExposeDataFolder:         "",
		ErrorMessageTimeout:      60,
		ErrorMessageRetries:      10,

		StartSubREQErrorLog:        true,
		StartSubREQHello:           true,
		StartSubREQToFileAppend:    true,
		StartSubREQToFile:          true,
		StartSubREQPing:            true,
		StartSubREQPong:            true,
		StartSubREQCliCommand:      true,
		StartSubREQnCliCommand:     true,
		StartSubREQToConsole:       true,
		StartSubREQHttpGet:         true,
		StartSubREQTailFile:        true,
		StartSubREQnCliCommandCont: true,
	}
	return c
}

// Check if all values are present in config file, and if not
// found use the default value.
func checkConfigValues(cf ConfigurationFromFile) Configuration {
	var conf Configuration
	cd := newConfigurationDefaults()

	if cf.ConfigFolder == nil {
		conf.ConfigFolder = cd.ConfigFolder
	} else {
		conf.ConfigFolder = *cf.ConfigFolder
	}
	if cf.SocketFolder == nil {
		conf.SocketFolder = cd.SocketFolder
	} else {
		conf.SocketFolder = *cf.SocketFolder
	}
	if cf.TCPListener == nil {
		conf.TCPListener = cd.TCPListener
	} else {
		conf.TCPListener = *cf.TCPListener
	}
	if cf.DatabaseFolder == nil {
		conf.DatabaseFolder = cd.DatabaseFolder
	} else {
		conf.DatabaseFolder = *cf.DatabaseFolder
	}
	if cf.NodeName == nil {
		conf.NodeName = cd.NodeName
	} else {
		conf.NodeName = *cf.NodeName
	}
	if cf.BrokerAddress == nil {
		conf.BrokerAddress = cd.BrokerAddress
	} else {
		conf.BrokerAddress = *cf.BrokerAddress
	}
	if cf.NatsConnectRetryInterval == nil {
		conf.NatsConnectRetryInterval = cd.NatsConnectRetryInterval
	} else {
		conf.NatsConnectRetryInterval = *cf.NatsConnectRetryInterval
	}
	if cf.ProfilingPort == nil {
		conf.ProfilingPort = cd.ProfilingPort
	} else {
		conf.ProfilingPort = *cf.ProfilingPort
	}
	if cf.PromHostAndPort == nil {
		conf.PromHostAndPort = cd.PromHostAndPort
	} else {
		conf.PromHostAndPort = *cf.PromHostAndPort
	}
	if cf.DefaultMessageTimeout == nil {
		conf.DefaultMessageTimeout = cd.DefaultMessageTimeout
	} else {
		conf.DefaultMessageTimeout = *cf.DefaultMessageTimeout
	}
	if cf.DefaultMessageRetries == nil {
		conf.DefaultMessageRetries = cd.DefaultMessageRetries
	} else {
		conf.DefaultMessageRetries = *cf.DefaultMessageRetries
	}
	if cf.SubscribersDataFolder == nil {
		conf.SubscribersDataFolder = cd.SubscribersDataFolder
	} else {
		conf.SubscribersDataFolder = *cf.SubscribersDataFolder
	}
	if cf.CentralNodeName == nil {
		conf.CentralNodeName = cd.CentralNodeName
	} else {
		conf.CentralNodeName = *cf.CentralNodeName
	}
	if cf.RootCAPath == nil {
		conf.RootCAPath = cd.RootCAPath
	} else {
		conf.RootCAPath = *cf.RootCAPath
	}
	if cf.NkeySeedFile == nil {
		conf.NkeySeedFile = cd.NkeySeedFile
	} else {
		conf.NkeySeedFile = *cf.NkeySeedFile
	}
	if cf.ExposeDataFolder == nil {
		conf.ExposeDataFolder = cd.ExposeDataFolder
	} else {
		conf.ExposeDataFolder = *cf.ExposeDataFolder
	}
	if cf.ErrorMessageTimeout == nil {
		conf.ErrorMessageTimeout = cd.ErrorMessageTimeout
	} else {
		conf.ErrorMessageTimeout = *cf.ErrorMessageTimeout
	}
	if cf.ErrorMessageRetries == nil {
		conf.ErrorMessageRetries = cd.ErrorMessageRetries
	} else {
		conf.ErrorMessageRetries = *cf.ErrorMessageRetries
	}
	if cf.StartPubREQHello == nil {
		conf.StartPubREQHello = cd.StartPubREQHello
	} else {
		conf.StartPubREQHello = *cf.StartPubREQHello
	}
	if cf.StartSubREQErrorLog == nil {
		conf.StartSubREQErrorLog = cd.StartSubREQErrorLog
	} else {
		conf.StartSubREQErrorLog = *cf.StartSubREQErrorLog
	}
	if cf.StartSubREQHello == nil {
		conf.StartSubREQHello = cd.StartSubREQHello
	} else {
		conf.StartSubREQHello = *cf.StartSubREQHello
	}
	if cf.StartSubREQToFileAppend == nil {
		conf.StartSubREQToFileAppend = cd.StartSubREQToFileAppend
	} else {
		conf.StartSubREQToFileAppend = *cf.StartSubREQToFileAppend
	}
	if cf.StartSubREQToFile == nil {
		conf.StartSubREQToFile = cd.StartSubREQToFile
	} else {
		conf.StartSubREQToFile = *cf.StartSubREQToFile
	}
	if cf.StartSubREQPing == nil {
		conf.StartSubREQPing = cd.StartSubREQPing
	} else {
		conf.StartSubREQPing = *cf.StartSubREQPing
	}
	if cf.StartSubREQPong == nil {
		conf.StartSubREQPong = cd.StartSubREQPong
	} else {
		conf.StartSubREQPong = *cf.StartSubREQPong
	}
	if cf.StartSubREQCliCommand == nil {
		conf.StartSubREQCliCommand = cd.StartSubREQCliCommand
	} else {
		conf.StartSubREQCliCommand = *cf.StartSubREQCliCommand
	}
	if cf.StartSubREQnCliCommand == nil {
		conf.StartSubREQnCliCommand = cd.StartSubREQnCliCommand
	} else {
		conf.StartSubREQnCliCommand = *cf.StartSubREQnCliCommand
	}
	if cf.StartSubREQToConsole == nil {
		conf.StartSubREQToConsole = cd.StartSubREQToConsole
	} else {
		conf.StartSubREQToConsole = *cf.StartSubREQToConsole
	}
	if cf.StartSubREQHttpGet == nil {
		conf.StartSubREQHttpGet = cd.StartSubREQHttpGet
	} else {
		conf.StartSubREQHttpGet = *cf.StartSubREQHttpGet
	}
	if cf.StartSubREQTailFile == nil {
		conf.StartSubREQTailFile = cd.StartSubREQTailFile
	} else {
		conf.StartSubREQTailFile = *cf.StartSubREQTailFile
	}
	if cf.StartSubREQnCliCommandCont == nil {
		conf.StartSubREQnCliCommandCont = cd.StartSubREQnCliCommandCont
	} else {
		conf.StartSubREQnCliCommandCont = *cf.StartSubREQnCliCommandCont
	}

	return conf
}

// CheckFlags will parse all flags
func (c *Configuration) CheckFlags() error {

	// Create an empty default config
	var fc Configuration

	// Set default configfolder if no env was provided.
	configFolder := os.Getenv("CONFIG_FOLDER")

	if configFolder == "" {
		configFolder = "./etc/"
	}

	// Read file config. Set system default if it can't find config file.
	fc, err := c.ReadConfigFile(configFolder)
	if err != nil {
		log.Printf("%v\n", err)
		fc = newConfigurationDefaults()
	}

	if configFolder == "" {
		fc.ConfigFolder = "./etc/"
	} else {
		fc.ConfigFolder = configFolder
	}

	*c = fc

	//flag.StringVar(&c.ConfigFolder, "configFolder", fc.ConfigFolder, "Defaults to ./usr/local/steward/etc/. *NB* This flag is not used, if your config file are located somwhere else than default set the location in an env variable named CONFIGFOLDER")
	flag.StringVar(&c.SocketFolder, "socketFolder", fc.SocketFolder, "folder who contains the socket file. Defaults to ./tmp/. If other folder is used this flag must be specified at startup.")
	flag.StringVar(&c.TCPListener, "tcpListener", fc.TCPListener, "start up a TCP listener in addition to the Unix Socket, to give messages to the system. e.g. localhost:8888. No value means not to start the listener, which is default. NB: You probably don't want to start this on any other interface than localhost")
	flag.StringVar(&c.DatabaseFolder, "databaseFolder", fc.DatabaseFolder, "folder who contains the database file. Defaults to ./var/lib/. If other folder is used this flag must be specified at startup.")
	flag.StringVar(&c.NodeName, "nodeName", fc.NodeName, "some unique string to identify this Edge unit")
	flag.StringVar(&c.BrokerAddress, "brokerAddress", fc.BrokerAddress, "the address of the message broker")
	flag.IntVar(&c.NatsConnectRetryInterval, "natsConnectRetryInterval", fc.NatsConnectRetryInterval, "default nats retry connect interval in seconds.")
	flag.StringVar(&c.ProfilingPort, "profilingPort", fc.ProfilingPort, "The number of the profiling port")
	flag.StringVar(&c.PromHostAndPort, "promHostAndPort", fc.PromHostAndPort, "host and port for prometheus listener, e.g. localhost:2112")
	flag.IntVar(&c.DefaultMessageTimeout, "defaultMessageTimeout", fc.DefaultMessageTimeout, "default message timeout in seconds. This can be overridden on the message level")
	flag.IntVar(&c.DefaultMessageRetries, "defaultMessageRetries", fc.DefaultMessageRetries, "default amount of retries that will be done before a message is thrown away, and out of the system")
	flag.StringVar(&c.SubscribersDataFolder, "subscribersDataFolder", fc.SubscribersDataFolder, "The data folder where subscribers are allowed to write their data if needed")
	flag.StringVar(&c.CentralNodeName, "centralNodeName", fc.CentralNodeName, "The name of the central node to receive messages published by this node")
	flag.StringVar(&c.RootCAPath, "rootCAPath", fc.RootCAPath, "If TLS, enter the path for where to find the root CA certificate")
	flag.StringVar(&c.NkeySeedFile, "nkeySeedFile", fc.NkeySeedFile, "The full path of the nkeys seed file")
	flag.StringVar(&c.ExposeDataFolder, "exposeDataFolder", fc.ExposeDataFolder, "If set the data folder will be exposed on the given host:port. Default value is not exposed at all")
	flag.IntVar(&c.ErrorMessageTimeout, "errorMessageTimeout", fc.ErrorMessageTimeout, "The number of seconds to wait for an error message to time out")
	flag.IntVar(&c.ErrorMessageRetries, "errorMessageRetries", fc.ErrorMessageRetries, "The number of if times to retry an error message before we drop it")

	flag.IntVar(&c.StartPubREQHello, "startPubREQHello", fc.StartPubREQHello, "Make the current node send hello messages to central at given interval in seconds")

	flag.BoolVar(&c.StartSubREQErrorLog, "startSubREQErrorLog", fc.StartSubREQErrorLog, "true/false")
	flag.BoolVar(&c.StartSubREQHello, "startSubREQHello", fc.StartSubREQHello, "true/false")
	flag.BoolVar(&c.StartSubREQToFileAppend, "startSubREQToFileAppend", fc.StartSubREQToFileAppend, "true/false")
	flag.BoolVar(&c.StartSubREQToFile, "startSubREQToFile", fc.StartSubREQToFile, "true/false")
	flag.BoolVar(&c.StartSubREQPing, "startSubREQPing", fc.StartSubREQPing, "true/false")
	flag.BoolVar(&c.StartSubREQPong, "startSubREQPong", fc.StartSubREQPong, "true/false")
	flag.BoolVar(&c.StartSubREQCliCommand, "startSubREQCliCommand", fc.StartSubREQCliCommand, "true/false")
	flag.BoolVar(&c.StartSubREQnCliCommand, "startSubREQnCliCommand", fc.StartSubREQnCliCommand, "true/false")
	flag.BoolVar(&c.StartSubREQToConsole, "startSubREQToConsole", fc.StartSubREQToConsole, "true/false")
	flag.BoolVar(&c.StartSubREQHttpGet, "startSubREQHttpGet", fc.StartSubREQHttpGet, "true/false")
	flag.BoolVar(&c.StartSubREQTailFile, "startSubREQTailFile", fc.StartSubREQTailFile, "true/false")
	flag.BoolVar(&c.StartSubREQnCliCommandCont, "startSubREQnCliCommandCont", fc.StartSubREQnCliCommandCont, "true/false")

	flag.Parse()

	// Check that mandatory flag values have been set.
	switch {
	case c.NodeName == "":
		return fmt.Errorf("error: the nodeName config option or flag cannot be empty, check -help")
	case c.CentralNodeName == "":
		return fmt.Errorf("error: the centralNodeName config option or flag cannot be empty, check -help")
	}

	if err := c.WriteConfigFile(); err != nil {
		log.Printf("error: checkFlags: failed writing config file: %v\n", err)
		os.Exit(1)
	}

	return nil
}

// Reads the current config file from disk.
func (c *Configuration) ReadConfigFile(configFolder string) (Configuration, error) {
	fPath := filepath.Join(configFolder, "config.toml")

	if _, err := os.Stat(fPath); os.IsNotExist(err) {
		return Configuration{}, fmt.Errorf("error: no config file found %v: %v", fPath, err)
	}

	f, err := os.OpenFile(fPath, os.O_RDONLY, 0600)
	if err != nil {
		return Configuration{}, fmt.Errorf("error: ReadConfigFile: failed to open file: %v", err)
	}
	defer f.Close()

	var cFile ConfigurationFromFile
	dec := toml.NewDecoder(f)
	err = dec.Decode(&cFile)
	if err != nil {
		log.Printf("error: decoding config.toml file. The program will automatically try to correct the problem, and use sane default where it kind find a value to use, but beware of this error in case the program start to behave in not expected ways: path=%v: err=%v", fPath, err)
	}

	// Check that all values read are ok.
	conf := checkConfigValues(cFile)

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
