package node

import (
	"github.com/p9c/pod/cmd/kopach/control"
	qu "github.com/p9c/pod/pkg/util/qu"
	"net"
	"net/http"
	// // This enables pprof
	// _ "net/http/pprof"
	"os"
	"runtime/pprof"
	
	"github.com/p9c/pod/pkg/util/logi"
	
	"github.com/p9c/pod/app/apputil"
	"github.com/p9c/pod/app/conte"
	"github.com/p9c/pod/cmd/node/path"
	indexers "github.com/p9c/pod/pkg/blockchain/indexers"
	database "github.com/p9c/pod/pkg/database"
	"github.com/p9c/pod/pkg/database/blockdb"
	"github.com/p9c/pod/pkg/rpc/chainrpc"
	"github.com/p9c/pod/pkg/util/interrupt"
)

// winServiceMain is only invoked on Windows. It detects when pod is running as
// a service and reacts accordingly.
var winServiceMain func() (bool, error)

// Main is the real main function for pod. It is necessary to work around the
// fact that deferred functions do not run when os.Exit() is called. The
// optional serverChan parameter is mainly used by the service code to be
// notified with the server once it is setup so it can gracefully stop it when
// requested from the service control manager.
func Main(cx *conte.Xt) (e error) {
	trc.Ln("starting up node main")
	// cx.WaitGroup.Add(1)
	cx.WaitAdd()
	// show version at startup
	// enable http profiling server if requested
	if *cx.Config.Profile != "" {
		dbg.Ln("profiling requested")
		go func() {
			listenAddr := net.JoinHostPort("", *cx.Config.Profile)
			inf.Ln("profile server listening on", listenAddr)
			profileRedirect := http.RedirectHandler("/debug/pprof", http.StatusSeeOther)
			http.Handle("/", profileRedirect)
			dbg.Ln("profile server", http.ListenAndServe(listenAddr, nil))
		}()
	}
	// write cpu profile if requested
	if *cx.Config.CPUProfile != "" && os.Getenv("POD_TRACE") != "on" {
		dbg.Ln("cpu profiling enabled")
		var f *os.File
		f, e = os.Create(*cx.Config.CPUProfile)
		if e != nil {
			err.Ln("unable to create cpu profile:", err)
			return
		}
		e := pprof.StartCPUProfile(f)
		if e != nil {
			dbg.Ln("failed to start up cpu profiler:", e)
		} else {
			defer func() {
				if e := f.Close(); err.Chk(e) {
				}
			}()
			defer pprof.StopCPUProfile()
			interrupt.AddHandler(
				func() {
					dbg.Ln("stopping CPU profiler")
					e := f.Close()
					if e != nil {
					}
					pprof.StopCPUProfile()
					dbg.Ln("finished cpu profiling", *cx.Config.CPUProfile)
				},
			)
		}
	}
	// perform upgrades to pod as new versions require it
	if e = doUpgrades(cx); err.Chk(e) {
		return
	}
	// return now if an interrupt signal was triggered
	if interrupt.Requested() {
		return nil
	}
	// load the block database
	var db database.DB
	db, e = loadBlockDB(cx)
	if e != nil {
		return
	}
	closeDb := func() {
		// ensure the database is synced and closed on shutdown
		trc.Ln("gracefully shutting down the database")
		func() {
			if e := db.Close(); err.Chk(e) {
			}
		}()
	}
	defer closeDb()
	interrupt.AddHandler(closeDb)
	// return now if an interrupt signal was triggered
	if interrupt.Requested() {
		return nil
	}
	// drop indexes and exit if requested. NOTE: The order is important here because
	// dropping the tx index also drops the address index since it relies on it
	if cx.StateCfg.DropAddrIndex {
		wrn.Ln("dropping address index")
		if e = indexers.DropAddrIndex(db, interrupt.ShutdownRequestChan); err.Chk(e) {
			return
		}
	}
	if cx.StateCfg.DropTxIndex {
		wrn.Ln("dropping transaction index")
		if e = indexers.DropTxIndex(db, interrupt.ShutdownRequestChan); err.Chk(e) {
			return
		}
	}
	if cx.StateCfg.DropCfIndex {
		wrn.Ln("dropping cfilter index")
		if e = indexers.DropCfIndex(db, interrupt.ShutdownRequestChan); err.Chk(e) {
			return
		}
	}
	// return now if an interrupt signal was triggered
	if interrupt.Requested() {
		return nil
	}
	mempoolUpdateChan := qu.Ts(1)
	mempoolUpdateHook := func() {
		mempoolUpdateChan.Signal()
	}
	// create server and start it
	var server *chainrpc.Node
	server, e = chainrpc.NewNode(
		*cx.Config.P2PListeners,
		db,
		interrupt.ShutdownRequestChan,
		conte.GetContext(cx),
		mempoolUpdateHook,
	)
	if e != nil {
		err.F("unable to start server on %v: %v", *cx.Config.P2PListeners, e)
		return e
	}
	server.Start()
	cx.RealNode = server
	if len(server.RPCServers) > 0 && *cx.Config.CAPI {
		dbg.Ln("starting cAPI.....")
		// chainrpc.RunAPI(server.RPCServers[0], cx.NodeKill)
		// dbg.Ln("propagating rpc server handle (node has started)")
	}
	if len(server.RPCServers) > 0 {
		cx.RPCServer = server.RPCServers[0]
		dbg.Ln("sending back node")
		cx.NodeChan <- cx.RPCServer
	}
	if !*cx.Config.DisableController {
		dbg.Ln("starting controller")
		cx.Controller = control.New(
			cx.Syncing,
			cx.Config,
			cx.StateCfg,
			cx.RealNode,
			cx.RPCServer,
			&cx.OtherNodesCounter,
			mempoolUpdateChan,
			uint64(*cx.Config.UUID),
			cx.KillAll,
		)
		go cx.Controller.Run()
		// cx.Controller.Start()
		dbg.Ln("controller started")
	}
	once := true
	gracefulShutdown := func() {
		if !once {
			return
		}
		if once {
			once = false
		}
		dbg.Ln("gracefully shutting down the server...")
		dbg.Ln("stopping controller")
		cx.Controller.Shutdown()
		dbg.Ln("stopping server")
		e := server.Stop()
		if e != nil {
			wrn.Ln("failed to stop server", e)
		}
		server.WaitForShutdown()
		inf.Ln("server shutdown complete")
		logi.L.LogChanDisabled.Store(true)
		logi.L.Writer.Write.Store(true)
		cx.WaitDone()
		cx.KillAll.Q()
		cx.NodeKill.Q()
	}
	dbg.Ln("adding interrupt handler for node")
	interrupt.AddHandler(gracefulShutdown)
	// Wait until the interrupt signal is received from an OS signal or shutdown is requested through one of the
	// subsystems such as the RPC server.
	select {
	case <-cx.NodeKill.Wait():
		dbg.Ln("NodeKill")
		if !interrupt.Requested() {
			interrupt.Request()
		}
		break
	case <-cx.KillAll.Wait():
		dbg.Ln("KillAll")
		if !interrupt.Requested() {
			interrupt.Request()
		}
		break
	}
	gracefulShutdown()
	return nil
}

// loadBlockDB loads (or creates when needed) the block database taking into
// account the selected database backend and returns a handle to it. It also
// additional logic such warning the user if there are multiple databases which
// consume space on the file system and ensuring the regression test database is
// clean when in regression test mode.
func loadBlockDB(cx *conte.Xt) (db database.DB, e error) {
	// The memdb backend does not have a file path associated with it, so handle it
	// uniquely. We also don't want to worry about the multiple database type
	// warnings when running with the memory database.
	if *cx.Config.DbType == "memdb" {
		inf.Ln("creating block database in memory")
		if db, e = database.Create(*cx.Config.DbType); err.Chk(e) {
			return nil, e
		}
		return db, nil
	}
	warnMultipleDBs(cx)
	// The database name is based on the database type.
	dbPath := path.BlockDb(cx, *cx.Config.DbType, blockdb.NamePrefix)
	// The regression test is special in that it needs a clean database for each
	// run, so remove it now if it already exists.
	e = removeRegressionDB(cx, dbPath)
	if e != nil {
		dbg.Ln("failed to remove regression db:", e)
	}
	inf.F("loading block database from '%s'", dbPath)
	if db, e = database.Open(*cx.Config.DbType, dbPath, cx.ActiveNet.Net); err.Chk(e) {
		trc.Ln(e) // return the error if it's not because the database doesn't exist
		if dbErr, ok := e.(database.DBError); !ok || dbErr.ErrorCode !=
			database.ErrDbDoesNotExist {
			return nil, e
		}
		// create the db if it does not exist
		e = os.MkdirAll(*cx.Config.DataDir, 0700)
		if e != nil {
			return nil, e
		}
		db, e = database.Create(*cx.Config.DbType, dbPath, cx.ActiveNet.Net)
		if e != nil {
			return nil, e
		}
	}
	trc.Ln("block database loaded")
	return db, nil
}

// removeRegressionDB removes the existing regression test database if running
// in regression test mode and it already exists.
func removeRegressionDB(cx *conte.Xt, dbPath string) (e error) {
	// don't do anything if not in regression test mode
	if !((*cx.Config.Network)[0] == 'r') {
		return nil
	}
	// remove the old regression test database if it already exists
	fi, e := os.Stat(dbPath)
	if e == nil {
		inf.F("removing regression test database from '%s' %s", dbPath)
		if fi.IsDir() {
			if e = os.RemoveAll(dbPath); err.Chk(e) {
				return e
			}
		} else {
			if e = os.Remove(dbPath); err.Chk(e) {
				return e
			}
		}
	}
	return nil
}

// warnMultipleDBs shows a warning if multiple block database types are
// detected. This is not a situation most users want. It is handy for
// development however to support multiple side-by-side databases.
func warnMultipleDBs(cx *conte.Xt) {
	// This is intentionally not using the known db types which depend on the
	// database types compiled into the binary since we want to detect legacy db
	// types as well.
	dbTypes := []string{"ffldb", "leveldb", "sqlite"}
	duplicateDbPaths := make([]string, 0, len(dbTypes)-1)
	for _, dbType := range dbTypes {
		if dbType == *cx.Config.DbType {
			continue
		}
		// store db path as a duplicate db if it exists
		dbPath := path.BlockDb(cx, dbType, blockdb.NamePrefix)
		if apputil.FileExists(dbPath) {
			duplicateDbPaths = append(duplicateDbPaths, dbPath)
		}
	}
	// warn if there are extra databases
	if len(duplicateDbPaths) > 0 {
		selectedDbPath := path.BlockDb(cx, *cx.Config.DbType, blockdb.NamePrefix)
		wrn.F(
			"\nThere are multiple block chain databases using different"+
				" database types.\nYou probably don't want to waste disk"+
				" space by having more than one."+
				"\nYour current database is located at [%v]."+
				"\nThe additional database is located at %v",
			selectedDbPath,
			duplicateDbPaths,
		)
	}
}
