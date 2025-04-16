# In-memory TFTP Server

This is a simple in-memory TFTP server, implemented in Go.  It is
RFC1350-compliant, but doesn't implement the additions in later RFCs.  In
particular, options are not recognized.

# Usage

To start the server, navigate to `cmd` -> `tftp` and run `go run main.go` in your terminal. This will start a tftp server on port 69 of your local host. Variables at the top of the main function can be modified if you would like to deploy this in other locations. 

To send a file to store, first enable TFTP client in windows. This will allow you to use the built in TFTP client via powershell. Then, in a powershell window, enter `tftp -i "localhost:69" put [YOUR_FILE]` to put the file on the server. Swap `put` for `get` to retrieve it. For more information please review the [docs](https://learn.microsoft.com/en-us/windows-server/administration/windows-commands/tftp)

# Testing

To run the unit tests, similarly navigate to `cmd` -> `tftp` and run `go test`. In the current state, the tests offer approximatly 75% coverage of the code.

Each test spins up its own instance of a TFTP server on a random port between 1024 and 65565. Each client similarly spins up on a random port in the same range. This ensure there is no overlap between tests and makes adding new tests simple. 
