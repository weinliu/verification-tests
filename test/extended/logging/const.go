package logging

const (
	// the namespace where legacy clusterlogging and clusterlogforwarder are in
	loggingNS = "openshift-logging"
	// the namespace where cluster-logging-operator is in
	cloNS = "openshift-logging"
	// the namespace where loki-operator is in
	loNS = "openshift-operators-redhat"

	apiPath         = "/api/logs/v1/"
	queryPath       = "/loki/api/v1/query"
	queryRangePath  = "/loki/api/v1/query_range"
	labelsPath      = "/loki/api/v1/labels"
	labelValuesPath = "/loki/api/v1/label/%s/values"
	seriesPath      = "/loki/api/v1/series"
	tailPath        = "/loki/api/v1/tail"
	rulesPath       = "/loki/api/v1/rules"
	minioNS         = "minio-aosqe"
	minioSecret     = "minio-creds"

	javaExc = `com.google.devtools.search.cloud.feeder.MakeLog: RuntimeException: Run from this message!
  at com.my.app.Object.do$a1(MakeLog.java:50)
  at java.lang.Thing.call(Thing.java:10)
  at com.my.app.Object.help(MakeLog.java:40)
  at sun.javax.API.method(API.java:100)
  at com.jetty.Framework.main(MakeLog.java:30)
`

	complexJavaExc = `javax.servlet.ServletException: Something bad happened
    at com.example.myproject.OpenSessionInViewFilter.doFilter(OpenSessionInViewFilter.java:60)
    at org.mortbay.jetty.servlet.ServletHandler$CachedChain.doFilter(ServletHandler.java:1157)
    at com.example.myproject.ExceptionHandlerFilter.doFilter(ExceptionHandlerFilter.java:28)
    at org.mortbay.jetty.servlet.ServletHandler$CachedChain.doFilter(ServletHandler.java:1157)
    at com.example.myproject.OutputBufferFilter.doFilter(OutputBufferFilter.java:33)
    at org.mortbay.jetty.servlet.ServletHandler$CachedChain.doFilter(ServletHandler.java:1157)
    at org.mortbay.jetty.servlet.ServletHandler.handle(ServletHandler.java:388)
    at org.mortbay.jetty.security.SecurityHandler.handle(SecurityHandler.java:216)
    at org.mortbay.jetty.servlet.SessionHandler.handle(SessionHandler.java:182)
    at org.mortbay.jetty.handler.ContextHandler.handle(ContextHandler.java:765)
    at org.mortbay.jetty.webapp.WebAppContext.handle(WebAppContext.java:418)
    at org.mortbay.jetty.handler.HandlerWrapper.handle(HandlerWrapper.java:152)
    at org.mortbay.jetty.Server.handle(Server.java:326)
    at org.mortbay.jetty.HttpConnection.handleRequest(HttpConnection.java:542)
    at org.mortbay.jetty.HttpConnection$RequestHandler.content(HttpConnection.java:943)
    at org.mortbay.jetty.HttpParser.parseNext(HttpParser.java:756)
    at org.mortbay.jetty.HttpParser.parseAvailable(HttpParser.java:218)
    at org.mortbay.jetty.HttpConnection.handle(HttpConnection.java:404)
    at org.mortbay.jetty.bio.SocketConnector$Connection.run(SocketConnector.java:228)
    at org.mortbay.thread.QueuedThreadPool$PoolThread.run(QueuedThreadPool.java:582)
Caused by: com.example.myproject.MyProjectServletException
    at com.example.myproject.MyServlet.doPost(MyServlet.java:169)
    at javax.servlet.http.HttpServlet.service(HttpServlet.java:727)
    at javax.servlet.http.HttpServlet.service(HttpServlet.java:820)
    at org.mortbay.jetty.servlet.ServletHolder.handle(ServletHolder.java:511)
    at org.mortbay.jetty.servlet.ServletHandler$CachedChain.doFilter(ServletHandler.java:1166)
    at com.example.myproject.OpenSessionInViewFilter.doFilter(OpenSessionInViewFilter.java:30)
    ... 27 common frames omitted
`

	nestedJavaExc = `java.lang.RuntimeException: javax.mail.SendFailedException: Invalid Addresses;
  nested exception is:
com.sun.mail.smtp.SMTPAddressFailedException: 550 5.7.1 <[REDACTED_EMAIL_ADDRESS]>... Relaying denied

	at com.nethunt.crm.api.server.adminsync.AutomaticEmailFacade.sendWithSmtp(AutomaticEmailFacade.java:236)
	at com.nethunt.crm.api.server.adminsync.AutomaticEmailFacade.sendSingleEmail(AutomaticEmailFacade.java:285)
	at com.nethunt.crm.api.server.adminsync.AutomaticEmailFacade.lambda$sendSingleEmail$3(AutomaticEmailFacade.java:254)
	at java.util.Optional.ifPresent(Optional.java:159)
	at com.nethunt.crm.api.server.adminsync.AutomaticEmailFacade.sendSingleEmail(AutomaticEmailFacade.java:253)
	at com.nethunt.crm.api.server.adminsync.AutomaticEmailFacade.sendSingleEmail(AutomaticEmailFacade.java:249)
	at com.nethunt.crm.api.email.EmailSender.lambda$notifyPerson$0(EmailSender.java:80)
	at com.nethunt.crm.api.util.ManagedExecutor.lambda$execute$0(ManagedExecutor.java:36)
	at com.nethunt.crm.api.util.RequestContextActivator.lambda$withRequestContext$0(RequestContextActivator.java:36)
	at java.base/java.util.concurrent.ThreadPoolExecutor.runWorker(ThreadPoolExecutor.java:1149)
	at java.base/java.util.concurrent.ThreadPoolExecutor$Worker.run(ThreadPoolExecutor.java:624)
	at java.base/java.lang.Thread.run(Thread.java:748)
Caused by: javax.mail.SendFailedException: Invalid Addresses;
  nested exception is:
com.sun.mail.smtp.SMTPAddressFailedException: 550 5.7.1 <[REDACTED_EMAIL_ADDRESS]>... Relaying denied

	at com.sun.mail.smtp.SMTPTransport.rcptTo(SMTPTransport.java:2064)
	at com.sun.mail.smtp.SMTPTransport.sendMessage(SMTPTransport.java:1286)
	at com.nethunt.crm.api.server.adminsync.AutomaticEmailFacade.sendWithSmtp(AutomaticEmailFacade.java:229)
	... 12 more
Caused by: com.sun.mail.smtp.SMTPAddressFailedException: 550 5.7.1 <[REDACTED_EMAIL_ADDRESS]>... Relaying denied
`

	nodeJsExc = `ReferenceError: myArray is not defined
  at next (/app/node_modules/express/lib/router/index.js:256:14)
  at /app/node_modules/express/lib/router/index.js:615:15
  at next (/app/node_modules/express/lib/router/index.js:271:10)
  at Function.process_params (/app/node_modules/express/lib/router/index.js:330:12)
  at /app/node_modules/express/lib/router/index.js:277:22
  at Layer.handle [as handle_request] (/app/node_modules/express/lib/router/layer.js:95:5)
  at Route.dispatch (/app/node_modules/express/lib/router/route.js:112:3)
  at next (/app/node_modules/express/lib/router/route.js:131:13)
  at Layer.handle [as handle_request] (/app/node_modules/express/lib/router/layer.js:95:5)
  at /app/app.js:52:3
`

	clientJsExc = `Error
    at bls (<anonymous>:3:9)
    at <anonymous>:6:4
    at a_function_name
    at Object.InjectedScript._evaluateOn (http://<anonymous>/file.js?foo=bar:875:140)
    at Object.InjectedScript.evaluate (<anonymous>)
`

	v8JsExc = `V8 errors stack trace
  eval at Foo.a (eval at Bar.z (myscript.js:10:3))
  at new Contructor.Name (native)
  at new FunctionName (unknown location)
  at Type.functionName [as methodName] (file(copy).js?query='yes':12:9)
  at functionName [as methodName] (native)
  at Type.main(sample(copy).js:6:4)
`

	pythonExc = `Traceback (most recent call last):
  File "/base/data/home/runtimes/python27/python27_lib/versions/third_party/webapp2-2.5.2/webapp2.py", line 1535, in __call__
    rv = self.handle_exception(request, response, e)
  File "/base/data/home/apps/s~nearfieldspy/1.378705245900539993/nearfieldspy.py", line 17, in start
    return get()
  File "/base/data/home/apps/s~nearfieldspy/1.378705245900539993/nearfieldspy.py", line 5, in get
    raise Exception('spam', 'eggs')
Exception: ('spam', 'eggs')
`

	phpExc = `exception 'Exception' with message 'Custom exception' in /home/joe/work/test-php/test.php:5
Stack trace:
#0 /home/joe/work/test-php/test.php(9): func1()
#1 /home/joe/work/test-php/test.php(13): func2()
#2 {main}
`

	phpOnGaeExc = `PHP Fatal error:  Uncaught exception 'Exception' with message 'message' in /base/data/home/apps/s~crash-example-php/1.388306779641080894/errors.php:60
Stack trace:
#0 [internal function]: ErrorEntryGenerator::{closure}()
#1 /base/data/home/apps/s~crash-example-php/1.388306779641080894/errors.php(20): call_user_func_array(Object(Closure), Array)
#2 /base/data/home/apps/s~crash-example-php/1.388306779641080894/index.php(36): ErrorEntry->__call('raise', Array)
#3 /base/data/home/apps/s~crash-example-php/1.388306779641080894/index.php(36): ErrorEntry->raise()
#4 {main}
  thrown in /base/data/home/apps/s~crash-example-php/1.388306779641080894/errors.php on line 60
`

	goExc = `panic: my panic

goroutine 4 [running]:
panic(0x45cb40, 0x47ad70)
	/usr/local/go/src/runtime/panic.go:542 +0x46c fp=0xc42003f7b8 sp=0xc42003f710 pc=0x422f7c
main.main.func1(0xc420024120)
	foo.go:6 +0x39 fp=0xc42003f7d8 sp=0xc42003f7b8 pc=0x451339
runtime.goexit()
	/usr/local/go/src/runtime/asm_amd64.s:2337 +0x1 fp=0xc42003f7e0 sp=0xc42003f7d8 pc=0x44b4d1
created by main.main
	foo.go:5 +0x58

goroutine 1 [chan receive]:
runtime.gopark(0x4739b8, 0xc420024178, 0x46fcd7, 0xc, 0xc420028e17, 0x3)
	/usr/local/go/src/runtime/proc.go:280 +0x12c fp=0xc420053e30 sp=0xc420053e00 pc=0x42503c
runtime.goparkunlock(0xc420024178, 0x46fcd7, 0xc, 0x1000f010040c217, 0x3)
	/usr/local/go/src/runtime/proc.go:286 +0x5e fp=0xc420053e70 sp=0xc420053e30 pc=0x42512e
runtime.chanrecv(0xc420024120, 0x0, 0xc420053f01, 0x4512d8)
	/usr/local/go/src/runtime/chan.go:506 +0x304 fp=0xc420053f20 sp=0xc420053e70 pc=0x4046b4
runtime.chanrecv1(0xc420024120, 0x0)
	/usr/local/go/src/runtime/chan.go:388 +0x2b fp=0xc420053f50 sp=0xc420053f20 pc=0x40439b
main.main()
	foo.go:9 +0x6f fp=0xc420053f80 sp=0xc420053f50 pc=0x4512ef
runtime.main()
	/usr/local/go/src/runtime/proc.go:185 +0x20d fp=0xc420053fe0 sp=0xc420053f80 pc=0x424bad
runtime.goexit()
	/usr/local/go/src/runtime/asm_amd64.s:2337 +0x1 fp=0xc420053fe8 sp=0xc420053fe0 pc=0x44b4d1

goroutine 2 [force gc (idle)]:
runtime.gopark(0x4739b8, 0x4ad720, 0x47001e, 0xf, 0x14, 0x1)
	/usr/local/go/src/runtime/proc.go:280 +0x12c fp=0xc42003e768 sp=0xc42003e738 pc=0x42503c
runtime.goparkunlock(0x4ad720, 0x47001e, 0xf, 0xc420000114, 0x1)
	/usr/local/go/src/runtime/proc.go:286 +0x5e fp=0xc42003e7a8 sp=0xc42003e768 pc=0x42512e
runtime.forcegchelper()
	/usr/local/go/src/runtime/proc.go:238 +0xcc fp=0xc42003e7e0 sp=0xc42003e7a8 pc=0x424e5c
runtime.goexit()
	/usr/local/go/src/runtime/asm_amd64.s:2337 +0x1 fp=0xc42003e7e8 sp=0xc42003e7e0 pc=0x44b4d1
created by runtime.init.4
	/usr/local/go/src/runtime/proc.go:227 +0x35

goroutine 3 [GC sweep wait]:
runtime.gopark(0x4739b8, 0x4ad7e0, 0x46fdd2, 0xd, 0x419914, 0x1)
	/usr/local/go/src/runtime/proc.go:280 +0x12c fp=0xc42003ef60 sp=0xc42003ef30 pc=0x42503c
runtime.goparkunlock(0x4ad7e0, 0x46fdd2, 0xd, 0x14, 0x1)
	/usr/local/go/src/runtime/proc.go:286 +0x5e fp=0xc42003efa0 sp=0xc42003ef60 pc=0x42512e
runtime.bgsweep(0xc42001e150)
	/usr/local/go/src/runtime/mgcsweep.go:52 +0xa3 fp=0xc42003efd8 sp=0xc42003efa0 pc=0x419973
runtime.goexit()
	/usr/local/go/src/runtime/asm_amd64.s:2337 +0x1 fp=0xc42003efe0 sp=0xc42003efd8 pc=0x44b4d1
created by runtime.gcenable
	/usr/local/go/src/runtime/mgc.go:216 +0x58
`

	goOnGaeExc = `panic: runtime error: index out of range

goroutine 12 [running]:
main88989.memoryAccessException()
	crash_example_go.go:58 +0x12a
main88989.handler(0x2afb7042a408, 0xc01042f880, 0xc0104d3450)
	crash_example_go.go:36 +0x7ec
net/http.HandlerFunc.ServeHTTP(0x13e5128, 0x2afb7042a408, 0xc01042f880, 0xc0104d3450)
	go/src/net/http/server.go:1265 +0x56
net/http.(*ServeMux).ServeHTTP(0xc01045cab0, 0x2afb7042a408, 0xc01042f880, 0xc0104d3450)
	go/src/net/http/server.go:1541 +0x1b4
appengine_internal.executeRequestSafely(0xc01042f880, 0xc0104d3450)
	go/src/appengine_internal/api_prod.go:288 +0xb7
appengine_internal.(*server).HandleRequest(0x15819b0, 0xc010401560, 0xc0104c8180, 0xc010431380, 0x0, 0x0)
	go/src/appengine_internal/api_prod.go:222 +0x102b
reflect.Value.call(0x1243fe0, 0x15819b0, 0x113, 0x12c8a20, 0x4, 0xc010485f78, 0x3, 0x3, 0x0, 0x0, ...)
	/tmp/appengine/go/src/reflect/value.go:419 +0x10fd
reflect.Value.Call(0x1243fe0, 0x15819b0, 0x113, 0xc010485f78, 0x3, 0x3, 0x0, 0x0, 0x0)
	/tmp/ap
`

	goSignalExc = `panic: runtime error: invalid memory address or nil pointer dereference
[signal SIGSEGV: segmentation violation code=0x1 addr=0x0 pc=0x7fd34f]

goroutine 5 [running]:
panics.nilPtrDereference()
	panics/panics.go:33 +0x1f
panics.Wait()
	panics/panics.go:16 +0x3b
created by main.main
	server.go:20 +0x91
`

	goHTTP = `http: panic serving [::1]:54143: test panic
goroutine 24 [running]:
net/http.(*conn).serve.func1(0xc00007eaa0)
	/usr/local/go/src/net/http/server.go:1746 +0xd0
panic(0x12472a0, 0x12ece10)
	/usr/local/go/src/runtime/panic.go:513 +0x1b9
main.doPanic(0x12f0ea0, 0xc00010e1c0, 0xc000104400)
	/Users/ingvar/src/go/src/httppanic.go:8 +0x39
net/http.HandlerFunc.ServeHTTP(0x12be2e8, 0x12f0ea0, 0xc00010e1c0, 0xc000104400)
	/usr/local/go/src/net/http/server.go:1964 +0x44
net/http.(*ServeMux).ServeHTTP(0x14a17a0, 0x12f0ea0, 0xc00010e1c0, 0xc000104400)
	/usr/local/go/src/net/http/server.go:2361 +0x127
net/http.serverHandler.ServeHTTP(0xc000085040, 0x12f0ea0, 0xc00010e1c0, 0xc000104400)
	/usr/local/go/src/net/http/server.go:2741 +0xab
net/http.(*conn).serve(0xc00007eaa0, 0x12f10a0, 0xc00008a780)
	/usr/local/go/src/net/http/server.go:1847 +0x646
created by net/http.(*Server).Serve
	/usr/local/go/src/net/http/server.go:2851 +0x2f5
`

	rubyExc = `NoMethodError (undefined method ` + "`" + `resursivewordload' for #<BooksController:0x007f8dd9a0c738>):
 app/controllers/books_controller.rb:69:in ` + "`" + `recursivewordload'
 app/controllers/books_controller.rb:75:in ` + "`" + `loadword'
 app/controllers/books_controller.rb:79:in ` + "`" + `loadline'
 app/controllers/books_controller.rb:83:in ` + "`" + `loadparagraph'
 app/controllers/books_controller.rb:87:in ` + "`" + `loadpage'
 app/controllers/books_controller.rb:91:in ` + "`" + `onload'
 app/controllers/books_controller.rb:95:in ` + "`" + `loadrecursive'
 app/controllers/books_controller.rb:99:in ` + "`" + `requestload'
 app/controllers/books_controller.rb:118:in ` + "`" + `generror'
 config/error_reporting_logger.rb:62:in ` + "`" + `tagged'
`

	//Please be careful when editing this file, there should have 2 blankspaces in the line below `RAILS_EXC = ` ActionController::RoutingError (No route matches [GET] "/settings"):`
	railsExc = ` ActionController::RoutingError (No route matches [GET] "/settings"):
  
  actionpack (5.1.4) lib/action_dispatch/middleware/debug_exceptions.rb:63:in ` + "`" + `call'
  actionpack (5.1.4) lib/action_dispatch/middleware/show_exceptions.rb:31:in ` + "`" + `call'
  railties (5.1.4) lib/rails/rack/logger.rb:36:in ` + "`" + `call_app'
  railties (5.1.4) lib/rails/rack/logger.rb:24:in ` + "`" + `block in call'
  activesupport (5.1.4) lib/active_support/tagged_logging.rb:69:in ` + "`" + `block in tagged'
  activesupport (5.1.4) lib/active_support/tagged_logging.rb:26:in ` + "`" + `tagged'
  activesupport (5.1.4) lib/active_support/tagged_logging.rb:69:in ` + "`" + `tagged'
  railties (5.1.4) lib/rails/rack/logger.rb:24:in ` + "`" + `call'
  actionpack (5.1.4) lib/action_dispatch/middleware/remote_ip.rb:79:in ` + "`" + `call'
  actionpack (5.1.4) lib/action_dispatch/middleware/request_id.rb:25:in ` + "`" + `call'
  rack (2.0.3) lib/rack/method_override.rb:22:in ` + "`" + `call'
  rack (2.0.3) lib/rack/runtime.rb:22:in ` + "`" + `call'
  activesupport (5.1.4) lib/active_support/cache/strategy/local_cache_middleware.rb:27:in ` + "`" + `call'
  actionpack (5.1.4) lib/action_dispatch/middleware/executor.rb:12:in ` + "`" + `call'
  rack (2.0.3) lib/rack/sendfile.rb:111:in ` + "`" + `call'
  railties (5.1.4) lib/rails/engine.rb:522:in ` + "`" + `call'
  puma (3.10.0) lib/puma/configuration.rb:225:in ` + "`" + `call'
  puma (3.10.0) lib/puma/server.rb:605:in ` + "`" + `handle_request'
  puma (3.10.0) lib/puma/server.rb:437:in ` + "`" + `process_client'
  puma (3.10.0) lib/puma/server.rb:301:in ` + "`" + `block in run'
  puma (3.10.0) lib/puma/thread_pool.rb:120:in ` + "`" + `block in spawn_thread'
`

	csharpExc = `System.Collections.Generic.KeyNotFoundException: The given key was not present in the dictionary.
  at System.Collections.Generic.Dictionary` + "`" + `2[System.String,System.Collections.Generic.Dictionary` + "`" + `2[System.Int32,System.Double]].get_Item (System.String key) [0x00000] in <filename unknown>:0
  at File3.Consolidator_Class.Function5 (System.Collections.Generic.Dictionary` + "`" + `2 names, System.Text.StringBuilder param_4) [0x00007] in /usr/local/google/home/Csharp/another file.csharp:9
  at File3.Consolidator_Class.Function4 (System.Text.StringBuilder param_4, System.Double[,,] array) [0x00013] in /usr/local/google/home/Csharp/another file.csharp:23
  at File3.Consolidator_Class.Function3 (Int32 param_3) [0x0000f] in /usr/local/google/home/Csharp/another file.csharp:27
  at File3.Consolidator_Class.Function3 (System.Text.StringBuilder param_3) [0x00007] in /usr/local/google/home/Csharp/another file.csharp:32
  at File2.Processor.Function2 (System.Int32& param_2, System.Collections.Generic.Stack` + "`" + `1& numbers) [0x00003] in /usr/local/google/home/Csharp/File2.csharp:19
  at File2.Processor.Random2 () [0x00037] in /usr/local/google/home/Csharp/File2.csharp:28
  at File2.Processor.Function1 (Int32 param_1, System.Collections.Generic.Dictionary` + "`" + `2 map) [0x00007] in /usr/local/google/home/Csharp/File2.csharp:34
  at Main.Welcome+<Main>c__AnonStorey0.<>m__0 () [0x00006] in /usr/local/google/home/Csharp/hello.csharp:48
  at System.Threading.Thread.StartInternal () [0x00000] in <filename unknown>:0
`

	csharpNestedExc = `System.InvalidOperationException: This is the outer exception ---> System.InvalidOperationException: This is the inner exception
  at ExampleApp.NestedExceptionExample.LowestLevelMethod() in c:/ExampleApp/ExampleApp/NestedExceptionExample.cs:line 33
  at ExampleApp.NestedExceptionExample.ThirdLevelMethod() in c:/ExampleApp/ExampleApp/NestedExceptionExample.cs:line 28
  at ExampleApp.NestedExceptionExample.SecondLevelMethod() in c:/ExampleApp/ExampleApp/NestedExceptionExample.cs:line 18
  --- End of inner exception stack trace ---
  at ExampleApp.NestedExceptionExample.SecondLevelMethod() in c:/ExampleApp/ExampleApp/NestedExceptionExample.cs:line 22
  at ExampleApp.NestedExceptionExample.TopLevelMethod() in c:/ExampleApp/ExampleApp/NestedExceptionExample.cs:line 11
  at ExampleApp.Program.Main(String[] args) in c:/ExampleApp/ExampleApp/Program.cs:line 11
`

	csharpAsyncExc = `System.InvalidOperationException: This is an exception
   at ExampleApp2.AsyncExceptionExample.LowestLevelMethod() in c:/ExampleApp/ExampleApp/AsyncExceptionExample.cs:line 36
   at ExampleApp2.AsyncExceptionExample.<ThirdLevelMethod>d__2.MoveNext() in c:/ExampleApp/ExampleApp/AsyncExceptionExample.cs:line 31
--- End of stack trace from previous location where exception was thrown ---
   at System.Runtime.CompilerServices.TaskAwaiter.ThrowForNonSuccess(Task task)
   at System.Runtime.CompilerServices.TaskAwaiter.HandleNonSuccessAndDebuggerNotification(Task task)
   at System.Runtime.CompilerServices.TaskAwaiter.GetResult()
   at ExampleApp2.AsyncExceptionExample.<SecondLevelMethod>d__1.MoveNext() in c:/ExampleApp/ExampleApp/AsyncExceptionExample.cs:line 25
--- End of stack trace from previous location where exception was thrown ---
   at System.Runtime.CompilerServices.TaskAwaiter.ThrowForNonSuccess(Task task)
   at System.Runtime.CompilerServices.TaskAwaiter.HandleNonSuccessAndDebuggerNotification(Task task)
   at System.Runtime.CompilerServices.TaskAwaiter.GetResult()
   at ExampleApp2.AsyncExceptionExample.<TopLevelMethod>d__0.MoveNext() in c:/ExampleApp/ExampleApp/AsyncExceptionExample.cs:line 14
`

	dartErr = `Unhandled exception:
Instance of 'MyError'
#0      main.<anonymous closure> (file:///path/to/code/dartFile.dart:15:20)
#1      printError (file:///path/to/code/dartFile.dart:37:13)
#2      main (file:///path/to/code/dartFile.dart:15:3)
#3      _startIsolate.<anonymous closure> (dart:isolate-patch/isolate_patch.dart:265)
#4      _RawReceivePortImpl._handleMessage (dart:isolate-patch/isolate_patch.dart:151)
`

	dartExc = `Unhandled exception:
Exception: exception message
#0      main.<anonymous closure> (file:///path/to/code/dartFile.dart:17:20)
#1      printError (file:///path/to/code/dartFile.dart:37:13)
#2      main (file:///path/to/code/dartFile.dart:17:3)
#3      _startIsolate.<anonymous closure> (dart:isolate-patch/isolate_patch.dart:265)
#4      _RawReceivePortImpl._handleMessage (dart:isolate-patch/isolate_patch.dart:151)
`

	dartAsyncErr = `Unhandled exception:
Bad state: oops
#0      handleFailure (file:///test/example/http/handling_an_httprequest_error.dart:16:3)
#1      main (file:///test/example/http/handling_an_httprequest_error.dart:24:5)
<asynchronous suspension>
#2      _startIsolate.<anonymous closure> (dart:isolate-patch/isolate_patch.dart:265)
#3      _RawReceivePortImpl._handleMessage (dart:isolate-patch/isolate_patch.dart:151)
`

	dartDivideByZeroErr = `Unhandled exception:
IntegerDivisionByZeroException
#0      main.<anonymous closure> (file:///path/to/code/dartFile.dart:27:20)
#1      printError (file:///path/to/code/dartFile.dart:42:13)
#2      main (file:///path/to/code/dartFile.dart:27:3)
#3      _startIsolate.<anonymous closure> (dart:isolate-patch/isolate_patch.dart:265)
#4      _RawReceivePortImpl._handleMessage (dart:isolate-patch/isolate_patch.dart:151)
`

	dartArgumentErr = `Unhandled exception:
Invalid argument(s): invalid argument
#0      main.<anonymous closure> (file:///path/to/code/dartFile.dart:23:20)
#1      printError (file:///path/to/code/dartFile.dart:42:13)
#2      main (file:///path/to/code/dartFile.dart:23:3)
#3      _startIsolate.<anonymous closure> (dart:isolate-patch/isolate_patch.dart:265)
#4      _RawReceivePortImpl._handleMessage (dart:isolate-patch/isolate_patch.dart:151)
`

	dartRangeErr = `Unhandled exception:
RangeError (index): Invalid value: Valid value range is empty: 1
#0      List.[] (dart:core-patch/growable_array.dart:151)
#1      main.<anonymous closure> (file:///path/to/code/dartFile.dart:31:23)
#2      printError (file:///path/to/code/dartFile.dart:42:13)
#3      main (file:///path/to/code/dartFile.dart:29:3)
#4      _startIsolate.<anonymous closure> (dart:isolate-patch/isolate_patch.dart:265)
#5      _RawReceivePortImpl._handleMessage (dart:isolate-patch/isolate_patch.dart:151)
`

	dartAssertionErr = `Unhandled exception:
Assertion failed
#0      main.<anonymous closure> (file:///path/to/code/dartFile.dart:9:20)
#1      printError (file:///path/to/code/dartFile.dart:36:13)
#2      main (file:///path/to/code/dartFile.dart:9:3)
#3      _startIsolate.<anonymous closure> (dart:isolate-patch/isolate_patch.dart:265)
#4      _RawReceivePortImpl._handleMessage (dart:isolate-patch/isolate_patch.dart:151)
`

	dartAbstractClassErr = `Unhandled exception:
Cannot instantiate abstract class LNClassName: _url 'null' line null
#0      main.<anonymous closure> (file:///path/to/code/dartFile.dart:12:20)
#1      printError (file:///path/to/code/dartFile.dart:36:13)
#2      main (file:///path/to/code/dartFile.dart:12:3)
#3      _startIsolate.<anonymous closure> (dart:isolate-patch/isolate_patch.dart:265)
#4      _RawReceivePortImpl._handleMessage (dart:isolate-patch/isolate_patch.dart:151)
`

	dartReadStaticErr = `Unhandled exception:
Reading static variable 'variable' during its initialization
#0      main.<anonymous closure> (file:///path/to/code/dartFile.dart:28:20)
#1      printError (file:///path/to/code/dartFile.dart:43:13)
#2      main (file:///path/to/code/dartFile.dart:28:3)
#3      _startIsolate.<anonymous closure> (dart:isolate-patch/isolate_patch.dart:265)
#4      _RawReceivePortImpl._handleMessage (dart:isolate-patch/isolate_patch.dart:151)
`

	dartUnimplementedErr = `Unhandled exception:
UnimplementedError: unimplemented
#0      main.<anonymous closure> (file:///path/to/code/dartFile.dart:38:20)
#1      printError (file:///path/to/code/dartFile.dart:61:13)
#2      main (file:///path/to/code/dartFile.dart:38:3)
#3      _startIsolate.<anonymous closure> (dart:isolate-patch/isolate_patch.dart:265)
#4      _RawReceivePortImpl._handleMessage (dart:isolate-patch/isolate_patch.dart:151)
`

	dartUnsupportedErr = `Unhandled exception:
Unsupported operation: unsupported
#0      main.<anonymous closure> (file:///path/to/code/dartFile.dart:36:20)
#1      printError (file:///path/to/code/dartFile.dart:61:13)
#2      main (file:///path/to/code/dartFile.dart:36:3)
#3      _startIsolate.<anonymous closure> (dart:isolate-patch/isolate_patch.dart:265)
#4      _RawReceivePortImpl._handleMessage (dart:isolate-patch/isolate_patch.dart:151)
`

	dartConcurrentModificationErr = `Unhandled exception:
Concurrent modification during iteration.
#0      main.<anonymous closure> (file:///path/to/code/dartFile.dart:35:20)
#1      printError (file:///path/to/code/dartFile.dart:61:13)
#2      main (file:///path/to/code/dartFile.dart:35:3)
#3      _startIsolate.<anonymous closure> (dart:isolate-patch/isolate_patch.dart:265)
#4      _RawReceivePortImpl._handleMessage (dart:isolate-patch/isolate_patch.dart:151)
`

	dartOOMErr = `Unhandled exception:
Out of Memory
#0      main.<anonymous closure> (file:///path/to/code/dartFile.dart:34:20)
#1      printError (file:///path/to/code/dartFile.dart:61:13)
#2      main (file:///path/to/code/dartFile.dart:34:3)
#3      _startIsolate.<anonymous closure> (dart:isolate-patch/isolate_patch.dart:265)
#4      _RawReceivePortImpl._handleMessage (dart:isolate-patch/isolate_patch.dart:151)
`

	dartStackOverflowErr = `Unhandled exception:
Stack Overflow
#0      main.<anonymous closure> (file:///path/to/code/dartFile.dart:33:20)
#1      printError (file:///path/to/code/dartFile.dart:61:13)
#2      main (file:///path/to/code/dartFile.dart:33:3)
#3      _startIsolate.<anonymous closure> (dart:isolate-patch/isolate_patch.dart:265)
#4      _RawReceivePortImpl._handleMessage (dart:isolate-patch/isolate_patch.dart:151)
`

	dartFallthroughErr = `Unhandled exception:
'null': Switch case fall-through at line null.
#0      main.<anonymous closure> (file:///path/to/code/dartFile.dart:39:20)
#1      printError (file:///path/to/code/dartFile.dart:51:13)
#2      main (file:///path/to/code/dartFile.dart:39:3)
#3      _startIsolate.<anonymous closure> (dart:isolate-patch/isolate_patch.dart:265)
#4      _RawReceivePortImpl._handleMessage (dart:isolate-patch/isolate_patch.dart:151)
`

	dartTypeErr = `Unhandled exception:
'file:///path/to/code/dartFile.dart': malformed type: line 7 pos 24: cannot resolve class 'NoType' from '::'
  printError( () { new NoType(); } );
                       ^


#0      _TypeError._throwNew (dart:core-patch/errors_patch.dart:82)
#1      main.<anonymous closure> (file:///path/to/code/dartFile.dart:7:24)
#2      printError (file:///path/to/code/dartFile.dart:36:13)
#3      main (file:///path/to/code/dartFile.dart:7:3)
#4      _startIsolate.<anonymous closure> (dart:isolate-patch/isolate_patch.dart:265)
#5      _RawReceivePortImpl._handleMessage (dart:isolate-patch/isolate_patch.dart:151)
`

	dartFormatErr = `Unhandled exception:
FormatException: format exception
#0      main.<anonymous closure> (file:///path/to/code/dartFile.dart:25:20)
#1      printError (file:///path/to/code/dartFile.dart:42:13)
#2      main (file:///path/to/code/dartFile.dart:25:3)
#3      _startIsolate.<anonymous closure> (dart:isolate-patch/isolate_patch.dart:265)
#4      _RawReceivePortImpl._handleMessage (dart:isolate-patch/isolate_patch.dart:151)
`

	dartFormatWithCodeErr = `Unhandled exception:
FormatException: Invalid base64 data (at line 3, character 8)
this is not valid
       ^

#0      main.<anonymous closure> (file:///path/to/code/dartFile.dart:24:20)
#1      printError (file:///path/to/code/dartFile.dart:42:13)
#2      main (file:///path/to/code/dartFile.dart:24:3)
#3      _startIsolate.<anonymous closure> (dart:isolate-patch/isolate_patch.dart:265)
#4      _RawReceivePortImpl._handleMessage (dart:isolate-patch/isolate_patch.dart:151)
`

	dartNoMethodErr = `Unhandled exception:
NoSuchMethodError: No constructor 'TypeError' with matching arguments declared in class 'TypeError'.
Receiver: Type: class 'TypeError'
Tried calling: new TypeError("Invalid base64 data", "invalid", 36)
Found: new TypeError()
#0      NoSuchMethodError._throwNew (dart:core-patch/errors_patch.dart:196)
#1      main.<anonymous closure> (file:///path/to/code/dartFile.dart:8:39)
#2      printError (file:///path/to/code/dartFile.dart:36:13)
#3      main (file:///path/to/code/dartFile.dart:8:3)
#4      _startIsolate.<anonymous closure> (dart:isolate-patch/isolate_patch.dart:265)
#5      _RawReceivePortImpl._handleMessage (dart:isolate-patch/isolate_patch.dart:151)
`

	dartNoMethodGlobalErr = `Unhandled exception:
NoSuchMethodError: No top-level method 'noMethod' declared.
Receiver: top-level
Tried calling: noMethod()
#0      NoSuchMethodError._throwNew (dart:core-patch/errors_patch.dart:196)
#1      main.<anonymous closure> (file:///path/to/code/dartFile.dart:10:20)
#2      printError (file:///path/to/code/dartFile.dart:36:13)
#3      main (file:///path/to/code/dartFile.dart:10:3)
#4      _startIsolate.<anonymous closure> (dart:isolate-patch/isolate_patch.dart:265)
#5      _RawReceivePortImpl._handleMessage (dart:isolate-patch/isolate_patch.dart:151)
`
)
