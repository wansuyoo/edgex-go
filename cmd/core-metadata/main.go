/*******************************************************************************
 * Copyright 2018 Dell Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software distributed under the License
 * is distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express
 * or implied. See the License for the specific language governing permissions and limitations under
 * the License.
 *******************************************************************************/
package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/edgexfoundry/edgex-go"
	"github.com/edgexfoundry/edgex-go/internal"
	"github.com/edgexfoundry/edgex-go/internal/core/metadata"
	"github.com/edgexfoundry/edgex-go/internal/pkg/usage"
	"github.com/edgexfoundry/edgex-go/pkg/clients/logging"
)

var bootTimeout int = 30000

func main() {
	start := time.Now()
	var useConsul bool
	var useProfile string

	flag.BoolVar(&useConsul, "consul", false, "Indicates the service should use consul.")
	flag.BoolVar(&useConsul, "c", false, "Indicates the service should use consul.")
	flag.StringVar(&useProfile, "profile", "", "Specify a profile other than default.")
	flag.StringVar(&useProfile, "p", "", "Specify a profile other than default.")
	flag.Usage = usage.HelpCallback
	flag.Parse()

	bootstrap(useConsul, useProfile)

	ok := metadata.Init()
	if !ok {
		logBeforeInit(fmt.Errorf("%s: Service bootstrap failed!", internal.CoreMetaDataServiceKey))
		return
	}

	metadata.LoggingClient.Info("Service dependencies resolved...")
	metadata.LoggingClient.Info(fmt.Sprintf("Starting %s %s ", internal.CoreMetaDataServiceKey, edgex.Version))

	http.TimeoutHandler(nil, time.Millisecond*time.Duration(metadata.Configuration.ServiceTimeout), "Request timed out")
	metadata.LoggingClient.Info(metadata.Configuration.AppOpenMsg, "")

	errs := make(chan error, 2)
	listenForInterrupt(errs)
	startHttpServer(errs, metadata.Configuration.ServicePort)

	// Time it took to start service
	metadata.LoggingClient.Info("Service started in: "+time.Since(start).String(), "")
	metadata.LoggingClient.Info("Listening on port: " + strconv.Itoa(metadata.Configuration.ServicePort))
	c := <-errs
	metadata.Destruct()
	metadata.LoggingClient.Warn(fmt.Sprintf("terminating: %v", c))
}

func logBeforeInit(err error) {
	l := logger.NewClient(internal.CoreMetaDataServiceKey, false, "")
	l.Error(err.Error())
}

func listenForInterrupt(errChan chan error) {
	go func() {
		c := make(chan os.Signal)
		signal.Notify(c, syscall.SIGINT)
		errChan <- fmt.Errorf("%s", <-c)
	}()
}

func startHttpServer(errChan chan error, port int) {
	go func() {
		r := metadata.LoadRestRoutes()
		errChan <- http.ListenAndServe(":"+strconv.Itoa(port), r)
	}()
}

func bootstrap(useConsul bool, useProfile string) {
	deps := make(chan error, 2)
	wg := sync.WaitGroup{}
	wg.Add(1)
	go metadata.ResolveDependencies(useConsul, useProfile, bootTimeout, &wg, deps)
	go func(ch chan error) {
		for {
			select {
			case e, ok := <-ch:
				if ok {
					logBeforeInit(e)
				} else {
					return
				}
			}
		}
	}(deps)

	wg.Wait()
}
