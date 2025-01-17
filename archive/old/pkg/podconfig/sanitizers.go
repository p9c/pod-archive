package podconfig

import (
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/p9c/pod/pkg/amt"
	"github.com/p9c/pod/pkg/chaincfg"
	"github.com/p9c/pod/pkg/fork"
	"github.com/p9c/pod/pkg/forkhash"
	"github.com/p9c/pod/pkg/logg"
	"github.com/p9c/pod/pkg/btcaddr"
	"github.com/p9c/pod/pkg/pod"
	"github.com/p9c/pod/pkg/podcfg"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	
	"github.com/p9c/pod/pkg/util/routeable"
	
	"github.com/p9c/pod/cmd/node"
	"github.com/p9c/pod/pkg/apputil"
	"github.com/p9c/pod/pkg/blockchain"
	"github.com/p9c/pod/pkg/connmgr"
	"github.com/p9c/pod/pkg/util"
	"github.com/p9c/pod/pkg/util/interrupt"
	"github.com/p9c/pod/pkg/util/normalize"
	"github.com/p9c/pod/pkg/wallet"
	
	"github.com/btcsuite/go-socks/socks"
	"github.com/urfave/cli"
	
	"github.com/p9c/pod/cmd/node/state"
	"github.com/p9c/pod/pkg/appdata"
)

const (
	appName           = "pod"
	confExt           = ".json"
	appLanguage       = "en"
	podConfigFilename = appName + confExt
	PARSER            = "json"
)

var funcName = "loadConfig"

func initDictionary(cfg *podcfg.Config) {
	if cfg.Language == nil || *cfg.Language == "" {
		*cfg.Language = Lang("en")
	}
	T.Ln("lang set to", *cfg.Language)
}

func initDataDir(cfg *podcfg.Config) {
	if cfg.DataDir == nil || *cfg.DataDir == "" {
		D.Ln("setting default data dir")
		*cfg.DataDir = appdata.Dir("pod", false)
	}
	T.Ln("datadir set to", *cfg.DataDir)
}

func initWalletFile(cx *pod.State) {
	if cx.Config.WalletFile == nil || *cx.Config.WalletFile == "" {
		*cx.Config.WalletFile = *cx.Config.DataDir + string(os.PathSeparator) +
			cx.ActiveNet.Name + string(os.PathSeparator) + wallet.DbName
	}
	T.Ln("wallet file set to", *cx.Config.WalletFile, *cx.Config.Network)
}

func initConfigFile(cfg *podcfg.Config) {
	if *cfg.ConfigFile == "" {
		*cfg.ConfigFile =
			*cfg.DataDir + string(os.PathSeparator) + podConfigFilename
	}
	T.Ln("using config file:", *cfg.ConfigFile)
}

func initLogDir(cfg *podcfg.Config) {
	if *cfg.LogDir != "" {
		// logi.L.SetLogPaths(*cfg.LogDir, "pod")
		interrupt.AddHandler(
			func() {
				D.Ln("initLogDir interrupt")
				// _ = logi.L.LogFileHandle.Close()
			},
		)
	}
}

func initParams(cx *pod.State) {
	network := "mainnet"
	if cx.Config.Network != nil {
		network = *cx.Config.Network
	}
	switch network {
	case "testnet", "testnet3", "t":
		T.Ln("on testnet")
		cx.ActiveNet = &chaincfg.TestNet3Params
		fork.IsTestnet = true
	case "regtestnet", "regressiontest", "r":
		T.Ln("on regression testnet")
		cx.ActiveNet = &chaincfg.RegressionTestParams
	case "simnet", "s":
		T.Ln("on simnet")
		cx.ActiveNet = &chaincfg.SimNetParams
	default:
		if network != "mainnet" && network != "m" {
			D.Ln("using mainnet for node")
		}
		T.Ln("on mainnet")
		cx.ActiveNet = &chaincfg.MainNetParams
	}
}

func validatePort(port string) bool {
	var e error
	var p int64
	if p, e = strconv.ParseInt(port, 10, 32); E.Chk(e) {
		return false
	}
	if p < 1024 || p > 65535 {
		return false
	}
	return true
}

func initListeners(cx *pod.State, commandName string, initial bool) {
	cfg := cx.Config
	var e error
	var fP int
	if fP, e = GetFreePort(); E.Chk(e) {
	}
	if *cfg.AutoListen {
		_, allAddresses := routeable.GetAddressesAndInterfaces()
		p2pAddresses := cli.StringSlice{}
		for addr := range allAddresses {
			p2pAddresses = append(p2pAddresses, net.JoinHostPort(addr, cx.ActiveNet.DefaultPort))
		}
		*cfg.P2PConnect = p2pAddresses
	}
	if len(*cfg.P2PListeners) < 1 && !*cfg.DisableListen && len(*cfg.ConnectPeers) < 1 {
		cfg.P2PListeners = &cli.StringSlice{fmt.Sprintf("0.0.0.0:" + cx.ActiveNet.DefaultPort)}
		cx.StateCfg.Save = true
		D.Ln("P2PListeners")
	}
	if len(*cfg.RPCListeners) < 1 {
		address := fmt.Sprintf("127.0.0.1:%s", cx.ActiveNet.RPCClientPort)
		*cfg.RPCListeners = cli.StringSlice{address}
		*cfg.RPCConnect = fmt.Sprintf("127.0.0.1:%s", cx.ActiveNet.RPCClientPort)
		D.Ln("setting save flag because rpc listeners is empty and rpc is not disabled")
		cx.StateCfg.Save = true
		D.Ln("RPCListeners")
	}
	if len(*cfg.WalletRPCListeners) < 1 && !*cfg.DisableRPC {
		address := fmt.Sprintf("127.0.0.1:" + cx.ActiveNet.WalletRPCServerPort)
		*cfg.WalletRPCListeners = cli.StringSlice{address}
		*cfg.WalletServer = address
		D.Ln(
			"setting save flag because wallet rpc listeners is empty and" +
				" rpc is not disabled",
		)
		cx.StateCfg.Save = true
		D.Ln("WalletRPCListeners")
	}
	if *cx.Config.AutoPorts || !initial {
		if fP, e = GetFreePort(); E.Chk(e) {
		}
		*cfg.P2PListeners = cli.StringSlice{"0.0.0.0:" + fmt.Sprint(fP)}
		if fP, e = GetFreePort(); E.Chk(e) {
		}
		*cfg.RPCListeners = cli.StringSlice{"127.0.0.1:" + fmt.Sprint(fP)}
		if fP, e = GetFreePort(); E.Chk(e) {
		}
		*cfg.WalletRPCListeners = cli.StringSlice{"127.0.0.1:" + fmt.Sprint(fP)}
		cx.StateCfg.Save = true
		D.Ln("autoports")
	} else {
		// sanitize user input and set auto on any that fail
		l := cfg.P2PListeners
		r := cfg.RPCListeners
		w := cfg.WalletRPCListeners
		for i := range *l {
			if _, p, e := net.SplitHostPort((*l)[i]); !E.Chk(e) {
				if !validatePort(p) {
					if fP, e = GetFreePort(); E.Chk(e) {
					}
					(*l)[i] = "0.0.0.0:" + fmt.Sprint(fP)
					cx.StateCfg.Save = true
					D.Ln("port not validate P2PListeners")
				}
			}
		}
		for i := range *r {
			if _, p, e := net.SplitHostPort((*r)[i]); !E.Chk(e) {
				if !validatePort(p) {
					if fP, e = GetFreePort(); E.Chk(e) {
					}
					(*r)[i] = "127.0.0.1:" + fmt.Sprint(fP)
					cx.StateCfg.Save = true
					D.Ln("port not validate RPCListeners")
				}
			}
		}
		for i := range *w {
			if _, p, e := net.SplitHostPort((*w)[i]); !E.Chk(e) {
				if !validatePort(p) {
					if fP, e = GetFreePort(); E.Chk(e) {
					}
					(*w)[i] = "127.0.0.1:" + fmt.Sprint(fP)
					cx.StateCfg.Save = true
					D.Ln("port not validate WalletRPCListeners")
				}
			}
		}
	}
	if *cfg.LAN && cx.ActiveNet.Name != "mainnet" {
		*cfg.DisableDNSSeed = true
	}
	if len(*cfg.WalletRPCListeners) > 0 {
		*cfg.WalletServer = (*cfg.WalletRPCListeners)[0]
	}
}

// GetFreePort asks the kernel for free open ports that are ready to use.
func GetFreePort() (int, error) {
	var port int
	addr, e := net.ResolveTCPAddr("tcp", "localhost:0")
	if e != nil {
		return 0, e
	}
	var l *net.TCPListener
	l, e = net.ListenTCP("tcp", addr)
	if e != nil {
		return 0, e
	}
	defer func() {
		if e := l.Close(); E.Chk(e) {
		}
	}()
	port = l.Addr().(*net.TCPAddr).Port
	return port, nil
}

func initTLSStuffs(cfg *podcfg.Config, st *state.Config) {
	isNew := false
	if *cfg.RPCCert == "" {
		*cfg.RPCCert =
			*cfg.DataDir + string(os.PathSeparator) + "rpc.cert"
		D.Ln("setting save flag because rpc cert path was not set")
		st.Save = true
		isNew = true
	}
	if *cfg.RPCKey == "" {
		*cfg.RPCKey =
			*cfg.DataDir + string(os.PathSeparator) + "rpc.key"
		D.Ln("setting save flag because rpc key path was not set")
		st.Save = true
		isNew = true
	}
	if *cfg.CAFile == "" {
		*cfg.CAFile =
			*cfg.DataDir + string(os.PathSeparator) + "ca.cert"
		D.Ln("setting save flag because CA cert path was not set")
		st.Save = true
		isNew = true
	}
	if isNew {
		// Now is the best time to make the certs
		I.Ln("generating TLS certificates")
		// Create directories for cert and key files if they do not yet exist.
		D.Ln("rpc tls ", *cfg.RPCCert, " ", *cfg.RPCKey)
		certDir, _ := filepath.Split(*cfg.RPCCert)
		keyDir, _ := filepath.Split(*cfg.RPCKey)
		var e error
		e = os.MkdirAll(certDir, 0700)
		if e != nil {
			E.Ln(e)
			return
		}
		e = os.MkdirAll(keyDir, 0700)
		if e != nil {
			E.Ln(e)
			return
		}
		// Generate cert pair.
		org := "pod/wallet autogenerated cert"
		validUntil := time.Now().Add(time.Hour * 24 * 365 * 10)
		cert, key, e := util.NewTLSCertPair(org, validUntil, nil)
		if e != nil {
			E.Ln(e)
			return
		}
		_, e = tls.X509KeyPair(cert, key)
		if e != nil {
			E.Ln(e)
			return
		}
		// Write cert and (potentially) the key files.
		e = ioutil.WriteFile(*cfg.RPCCert, cert, 0600)
		if e != nil {
			rmErr := os.Remove(*cfg.RPCCert)
			if rmErr != nil {
				E.Ln("cannot remove written certificates:", rmErr)
			}
			return
		}
		e = ioutil.WriteFile(*cfg.CAFile, cert, 0600)
		if e != nil {
			rmErr := os.Remove(*cfg.RPCCert)
			if rmErr != nil {
				E.Ln("cannot remove written certificates:", rmErr)
			}
			return
		}
		e = ioutil.WriteFile(*cfg.RPCKey, key, 0600)
		if e != nil {
			E.Ln(e)
			rmErr := os.Remove(*cfg.RPCCert)
			if rmErr != nil {
				E.Ln("cannot remove written certificates:", rmErr)
			}
			rmErr = os.Remove(*cfg.CAFile)
			if rmErr != nil {
				E.Ln("cannot remove written certificates:", rmErr)
			}
			return
		}
		I.Ln("done generating TLS certificates")
		return
	}
}

func initLogLevel(cfg *podcfg.Config) {
	loglevel := *cfg.LogLevel
	switch loglevel {
	case "trace", "debug", "info", "warn", "error", "fatal", "off":
		D.Ln("log level", loglevel)
	default:
		E.Ln("unrecognised loglevel", loglevel, "setting default info")
		*cfg.LogLevel = "info"
	}
	logg.SetLogLevel(*cfg.LogLevel)
}

func normalizeAddresses(cfg *podcfg.Config) {
	T.Ln("normalising addresses")
	port := node.DefaultPort
	nrm := normalize.StringSliceAddresses
	nrm(cfg.AddPeers, port)
	nrm(cfg.ConnectPeers, port)
	nrm(cfg.P2PListeners, port)
	nrm(cfg.Whitelists, port)
	// nrm(cfg.RPCListeners, port)
}

func setRelayReject(cfg *podcfg.Config) {
	relayNonStd := *cfg.RelayNonStd
	switch {
	case *cfg.RelayNonStd && *cfg.RejectNonStd:
		errf := "%s: rejectnonstd and relaynonstd cannot be used together" +
			" -- choose only one, leaving neither activated"
		E.Ln(errf, funcName)
		// just leave both false
		*cfg.RelayNonStd = false
		*cfg.RejectNonStd = false
	case *cfg.RejectNonStd:
		relayNonStd = false
	case *cfg.RelayNonStd:
		relayNonStd = true
	}
	*cfg.RelayNonStd = relayNonStd
}

func validateDBtype(cfg *podcfg.Config) {
	// Validate database type.
	T.Ln("validating database type")
	if !node.ValidDbType(*cfg.DbType) {
		str := "%s: The specified database type [%v] is invalid -- " +
			"supported types %v"
		e := fmt.Errorf(str, funcName, *cfg.DbType, node.KnownDbTypes)
		E.Ln(funcName, e)
		// set to default
		*cfg.DbType = node.KnownDbTypes[0]
	}
}

func validateProfilePort(cfg *podcfg.Config) {
	// Validate profile port number
	T.Ln("validating profile port number")
	if *cfg.Profile != "" {
		profilePort, e := strconv.Atoi(*cfg.Profile)
		if e != nil || profilePort < 1024 || profilePort > 65535 {
			str := "%s: The profile port must be between 1024 and 65535"
			e = fmt.Errorf(str, funcName)
			E.Ln(funcName, e)
			*cfg.Profile = ""
		}
	}
}
func validateBanDuration(cfg *podcfg.Config) {
	// Don't allow ban durations that are too short.
	T.Ln("validating ban duration")
	if *cfg.BanDuration < time.Second {
		e := fmt.Errorf(
			"%s: The banduration option may not be less than 1s -- parsed [%v]",
			funcName, *cfg.BanDuration,
		)
		I.Ln(funcName, e)
		*cfg.BanDuration = node.DefaultBanDuration
	}
}

func validateWhitelists(cfg *podcfg.Config, st *state.Config) {
	// Validate any given whitelisted IP addresses and networks.
	T.Ln("validating whitelists")
	if len(*cfg.Whitelists) > 0 {
		var ip net.IP
		st.ActiveWhitelists = make([]*net.IPNet, 0, len(*cfg.Whitelists))
		for _, addr := range *cfg.Whitelists {
			_, ipnet, e := net.ParseCIDR(addr)
			if e != nil {
				E.Ln(e)
				ip = net.ParseIP(addr)
				if ip == nil {
					str := e.Error() + " %s: The whitelist value of '%s' is invalid"
					e = fmt.Errorf(str, funcName, addr)
					E.Ln(e)
					_, _ = fmt.Fprintln(os.Stderr, e)
					interrupt.Request()
					// os.Exit(1)
				} else {
					var bits int
					if ip.To4() == nil {
						// IPv6
						bits = 128
					} else {
						bits = 32
					}
					ipnet = &net.IPNet{
						IP:   ip,
						Mask: net.CIDRMask(bits, bits),
					}
				}
			}
			st.ActiveWhitelists = append(st.ActiveWhitelists, ipnet)
		}
	}
}

func validatePeerLists(cfg *podcfg.Config) {
	T.Ln("checking addpeer and connectpeer lists")
	if len(*cfg.AddPeers) > 0 && len(*cfg.ConnectPeers) > 0 {
		e := fmt.Errorf(
			"%s: the --addpeer and --connect options can not be mixed",
			funcName,
		)
		_, _ = fmt.Fprintln(os.Stderr, e)
		// os.Exit(1)
	}
}
func configListener(cfg *podcfg.Config, params *chaincfg.Params) {
	// --proxy or --connect without --listen disables listening.
	T.Ln("checking proxy/connect for disabling listening")
	if (*cfg.Proxy != "" ||
		len(*cfg.ConnectPeers) > 0) &&
		len(*cfg.P2PListeners) == 0 {
		*cfg.DisableListen = true
	}
	// Add the default listener if none were specified. The default listener is all
	// addresses on the listen port for the network we are to connect to.
	T.Ln("checking if listener was set")
	if len(*cfg.P2PListeners) == 0 {
		*cfg.P2PListeners = []string{"0.0.0.0:" + params.DefaultPort}
	}
}

func validateUsers(cfg *podcfg.Config) {
	// Chk to make sure limited and admin users don't have the same username
	T.Ln("checking admin and limited username is different")
	if *cfg.Username != "" &&
		*cfg.Username == *cfg.LimitUser {
		str := "%s: --username and --limituser must not specify the same username"
		e := fmt.Errorf(str, funcName)
		_, _ = fmt.Fprintln(os.Stderr, e)
	}
	// Chk to make sure limited and admin users don't have the same password
	T.Ln("checking limited and admin passwords are not the same")
	if *cfg.Password != "" &&
		*cfg.Password == *cfg.LimitPass {
		str := "%s: --password and --limitpass must not specify the same password"
		e := fmt.Errorf(str, funcName)
		_, _ = fmt.Fprintln(os.Stderr, e)
		// os.Exit(1)
	}
}

func configRPC(cfg *podcfg.Config, params *chaincfg.Params) {
	// The RPC server is disabled if no username or password is provided.
	T.Ln("checking rpc server has a login enabled")
	if (*cfg.Username == "" || *cfg.Password == "") &&
		(*cfg.LimitUser == "" || *cfg.LimitPass == "") {
		*cfg.DisableRPC = true
	}
	if *cfg.DisableRPC {
		T.Ln("RPC service is disabled")
	}
	T.Ln("checking rpc server has listeners set")
	if !*cfg.DisableRPC && len(*cfg.RPCListeners) == 0 {
		D.Ln("looking up default listener")
		addrs, e := net.LookupHost(node.DefaultRPCListener)
		if e != nil {
			E.Ln(e)
			// os.Exit(1)
		}
		*cfg.RPCListeners = make([]string, 0, len(addrs))
		D.Ln("setting listeners")
		for _, addr := range addrs {
			*cfg.RPCListeners = append(*cfg.RPCListeners, addr)
			addr = net.JoinHostPort(addr, params.RPCClientPort)
		}
	}
	T.Ln("checking rpc max concurrent requests")
	if *cfg.RPCMaxConcurrentReqs < 0 {
		str := "%s: The rpcmaxwebsocketconcurrentrequests option may not be" +
			" less than 0 -- parsed [%d]"
		e := fmt.Errorf(str, funcName, *cfg.RPCMaxConcurrentReqs)
		_, _ = fmt.Fprintln(os.Stderr, e)
		// os.Exit(1)
	}
	T.Ln("checking rpc listener addresses")
	nrms := normalize.Addresses
	// Add default port to all added peer addresses if needed and remove duplicate addresses.
	*cfg.AddPeers = nrms(*cfg.AddPeers, params.DefaultPort)
	*cfg.ConnectPeers = nrms(*cfg.ConnectPeers, params.DefaultPort)
}

func validatePolicies(cfg *podcfg.Config, stateConfig *state.Config) {
	var e error
	// Validate the the minrelaytxfee.
	T.Ln("checking min relay tx fee")
	stateConfig.ActiveMinRelayTxFee, e = amt.NewAmount(*cfg.MinRelayTxFee)
	if e != nil {
		E.Ln(e)
		str := "%s: invalid minrelaytxfee: %v"
		e := fmt.Errorf(str, funcName, e)
		_, _ = fmt.Fprintln(os.Stderr, e)
	}
	// Limit the max block size to a sane value.
	T.Ln("checking max block size")
	if *cfg.BlockMaxSize < node.BlockMaxSizeMin ||
		*cfg.BlockMaxSize > node.BlockMaxSizeMax {
		str := "%s: The blockmaxsize option must be in between %d and %d -- parsed [%d]"
		e := fmt.Errorf(
			str, funcName, node.BlockMaxSizeMin,
			node.BlockMaxSizeMax, *cfg.BlockMaxSize,
		)
		_, _ = fmt.Fprintln(os.Stderr, e)
	}
	// Limit the max block weight to a sane value.
	T.Ln("checking max block weight")
	if *cfg.BlockMaxWeight < node.BlockMaxWeightMin ||
		*cfg.BlockMaxWeight > node.BlockMaxWeightMax {
		str := "%s: The blockmaxweight option must be in between %d and %d -- parsed [%d]"
		e := fmt.Errorf(
			str, funcName, node.BlockMaxWeightMin,
			node.BlockMaxWeightMax, *cfg.BlockMaxWeight,
		)
		_, _ = fmt.Fprintln(os.Stderr, e)
	}
	// Limit the max orphan count to a sane vlue.
	T.Ln("checking max orphan limit")
	if *cfg.MaxOrphanTxs < 0 {
		str := "%s: The maxorphantx option may not be less than 0 -- parsed [%d]"
		e := fmt.Errorf(str, funcName, *cfg.MaxOrphanTxs)
		_, _ = fmt.Fprintln(os.Stderr, e)
	}
	// Limit the block priority and minimum block txsizes to max block size.
	T.Ln("validating block priority and minimum size/weight")
	*cfg.BlockPrioritySize = int(
		apputil.MinUint32(
			uint32(*cfg.BlockPrioritySize),
			uint32(*cfg.BlockMaxSize),
		),
	)
	*cfg.BlockMinSize = int(
		apputil.MinUint32(
			uint32(*cfg.BlockMinSize),
			uint32(*cfg.BlockMaxSize),
		),
	)
	*cfg.BlockMinWeight = int(
		apputil.MinUint32(
			uint32(*cfg.BlockMinWeight),
			uint32(*cfg.BlockMaxWeight),
		),
	)
	switch {
	// If the max block size isn't set, but the max weight is, then we'll set the
	// limit for the max block size to a safe limit so weight takes precedence.
	case *cfg.BlockMaxSize == node.DefaultBlockMaxSize &&
		*cfg.BlockMaxWeight != node.DefaultBlockMaxWeight:
		*cfg.BlockMaxSize = blockchain.MaxBlockBaseSize - 1000
		// If the max block weight isn't set, but the block size is, then we'll scale
		// the set weight accordingly based on the max block size value.
	case *cfg.BlockMaxSize != node.DefaultBlockMaxSize &&
		*cfg.BlockMaxWeight == node.DefaultBlockMaxWeight:
		*cfg.BlockMaxWeight = *cfg.BlockMaxSize * blockchain.WitnessScaleFactor
	}
	// Look for illegal characters in the user agent comments.
	T.Ln("checking user agent comments", cfg.UserAgentComments)
	for _, uaComment := range *cfg.UserAgentComments {
		if strings.ContainsAny(uaComment, "/:()") {
			e := fmt.Errorf(
				"%s: The following characters must not "+
					"appear in user agent comments: '/', ':', '(', ')'",
				funcName,
			)
			_, _ = fmt.Fprintln(os.Stderr, e)
		}
	}
	// Chk the checkpoints for syntax errors.
	T.Ln("checking the checkpoints")
	stateConfig.AddedCheckpoints, e = node.ParseCheckpoints(
		*cfg.
			AddCheckpoints,
	)
	if e != nil {
		E.Ln(e)
		str := "%s: err parsing checkpoints: %v"
		e := fmt.Errorf(str, funcName, e)
		_, _ = fmt.Fprintln(os.Stderr, e)
	}
}
func validateOnions(cfg *podcfg.Config) {
	// --onionproxy and not --onion are contradictory
	// TODO: this is kinda stupid hm? switch *and* toggle by presence of flag value, one should be enough
	if *cfg.Onion && *cfg.OnionProxy != "" {
		E.Ln("onion enabled but no onionproxy has been configured")
		T.Ln("halting to avoid exposing IP address")
	}
	// Tor stream isolation requires either proxy or onion proxy to be set.
	if *cfg.TorIsolation &&
		*cfg.Proxy == "" &&
		*cfg.OnionProxy == "" {
		str := "%s: Tor stream isolation requires either proxy or onionproxy to be set"
		e := fmt.Errorf(str, funcName)
		_, _ = fmt.Fprintln(os.Stderr, e)
		// os.Exit(1)
	}
	if !*cfg.Onion {
		*cfg.OnionProxy = ""
	}
	
}

func validateMiningStuff(
	cfg *podcfg.Config, state *state.Config,
	params *chaincfg.Params,
) {
	if state == nil {
		panic("state is nil")
	}
	// Chk mining addresses are valid and saved parsed versions.
	T.Ln("checking mining addresses")
	aml := 99
	if cfg.MiningAddrs != nil {
		aml = len(*cfg.MiningAddrs)
	} else {
		D.Ln("MiningAddrs is nil")
		return
	}
	state.ActiveMiningAddrs = make([]btcaddr.Address, 0, aml)
	for _, strAddr := range *cfg.MiningAddrs {
		addr, e := btcaddr.Decode(strAddr, params)
		if e != nil {
			E.Ln(e)
			str := "%s: mining address '%s' failed to decode: %v"
			e := fmt.Errorf(str, funcName, strAddr, e)
			_, _ = fmt.Fprintln(os.Stderr, e)
			// os.Exit(1)
			continue
		}
		if !addr.IsForNet(params) {
			str := "%s: mining address '%s' is on the wrong network"
			e := fmt.Errorf(str, funcName, strAddr)
			_, _ = fmt.Fprintln(os.Stderr, e)
			// os.Exit(1)
			continue
		}
		state.ActiveMiningAddrs = append(state.ActiveMiningAddrs, addr)
	}
	if *cfg.MinerPass == "" {
		D.Ln("--------------- generating new miner key")
		*cfg.MinerPass = hex.EncodeToString(forkhash.Argon2i([]byte(*cfg.MinerPass)))
		state.ActiveMinerKey = []byte(*cfg.MinerPass)
	}
}

func setDiallers(cfg *podcfg.Config, stateConfig *state.Config) {
	// Setup dial and DNS resolution (lookup) functions depending on the specified
	// options. The default is to use the standard net.DialTimeout function as well
	// as the system DNS resolver. When a proxy is specified, the dial function is
	// set to the proxy specific dial function and the lookup is set to use tor
	// (unless --noonion is specified in which case the system DNS resolver is
	// used).
	T.Ln("setting network dialer and lookup")
	stateConfig.Dial = net.DialTimeout
	stateConfig.Lookup = net.LookupIP
	var e error
	if *cfg.Proxy != "" {
		T.Ln("we are loading a proxy!")
		_, _, e = net.SplitHostPort(*cfg.Proxy)
		if e != nil {
			E.Ln(e)
			str := "%s: Proxy address '%s' is invalid: %v"
			e = fmt.Errorf(str, funcName, *cfg.Proxy, e)
			fmt.Fprintln(os.Stderr, e)
			// os.Exit(1)
		}
		// Tor isolation flag means proxy credentials will be overridden unless there is
		// also an onion proxy configured in which case that one will be overridden.
		torIsolation := false
		if *cfg.TorIsolation &&
			*cfg.OnionProxy == "" &&
			(*cfg.ProxyUser != "" ||
				*cfg.ProxyPass != "") {
			torIsolation = true
			W.Ln(
				"Tor isolation set -- overriding specified" +
					" proxy user credentials",
			)
		}
		proxy := &socks.Proxy{
			Addr:         *cfg.Proxy,
			Username:     *cfg.ProxyUser,
			Password:     *cfg.ProxyPass,
			TorIsolation: torIsolation,
		}
		stateConfig.Dial = proxy.DialTimeout
		// Treat the proxy as tor and perform DNS resolution through it unless the
		// --noonion flag is set or there is an onion-specific proxy configured.
		if *cfg.Onion &&
			*cfg.OnionProxy == "" {
			stateConfig.Lookup = func(host string) ([]net.IP, error) {
				return connmgr.TorLookupIP(host, *cfg.Proxy)
			}
		}
	}
	// Setup onion address dial function depending on the specified options. The
	// default is to use the same dial function selected above. However, when an
	// onion-specific proxy is specified, the onion address dial function is set to
	// use the onion-specific proxy while leaving the normal dial function as
	// selected above. This allows .onion address traffic to be routed through a
	// different proxy than normal traffic.
	T.Ln("setting up tor proxy if enabled")
	if *cfg.OnionProxy != "" {
		_, _, e = net.SplitHostPort(*cfg.OnionProxy)
		if e != nil {
			E.Ln(e)
			str := "%s: Onion proxy address '%s' is invalid: %v"
			e = fmt.Errorf(str, funcName, *cfg.OnionProxy, e)
			_, _ = fmt.Fprintln(os.Stderr, e)
		}
		// Tor isolation flag means onion proxy credentials will be overridden.
		if *cfg.TorIsolation &&
			(*cfg.OnionProxyUser != "" || *cfg.OnionProxyPass != "") {
			W.Ln(
				"Tor isolation set - overriding specified onionproxy user" +
					" credentials",
			)
		}
	}
	T.Ln("setting onion dialer")
	stateConfig.Oniondial =
		func(network, addr string, timeout time.Duration) (net.Conn, error) {
			proxy := &socks.Proxy{
				Addr:         *cfg.OnionProxy,
				Username:     *cfg.OnionProxyUser,
				Password:     *cfg.OnionProxyPass,
				TorIsolation: *cfg.TorIsolation,
			}
			return proxy.DialTimeout(network, addr, timeout)
		}
	
	// When configured in bridge mode (both --onion and --proxy are configured), it
	// means that the proxy configured by --proxy is not a tor proxy, so override
	// the DNS resolution to use the onion-specific proxy.
	T.Ln("setting proxy lookup")
	if *cfg.Proxy != "" {
		stateConfig.Lookup = func(host string) ([]net.IP, error) {
			return connmgr.TorLookupIP(host, *cfg.OnionProxy)
		}
	} else {
		stateConfig.Oniondial = stateConfig.Dial
	}
	// Specifying --noonion means the onion address dial function results in an error.
	if !*cfg.Onion {
		stateConfig.Oniondial = func(a, b string, t time.Duration) (net.Conn, error) {
			return nil, errors.New("tor has been disabled")
		}
	}
}
