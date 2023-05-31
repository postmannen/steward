func (m methodREQToFile) handler(proc process, message Message, node string) ([]byte, error) {

	// If it was a request type message we want to check what the initial messages
	// method, so we can use that in creating the file name to store the data.
	fileName, folderTree := selectFileNaming(message, proc)

	// Check if folder structure exist, if not create it.
	if _, err := os.Stat(folderTree); os.IsNotExist(err) {
		err := os.MkdirAll(folderTree, 0770)
		if err != nil {
			er := fmt.Errorf("error: methodREQToFile failed to create toFile directory tree: subject:%v, folderTree: %v, %v", proc.subject, folderTree, err)
			proc.errorKernel.errSend(proc, message, er, logWarning)

			return nil, er
		}

		er := fmt.Errorf("info: Creating subscribers data folder at %v", folderTree)
		proc.errorKernel.logDebug(er, proc.configuration)
	}

	// Open file and write data.
	file := filepath.Join(folderTree, fileName)
	f, err := os.OpenFile(file, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0755)
	if err != nil {
		er := fmt.Errorf("error: methodREQToFile.handler: failed to open file, check that you've specified a value for fileName in the message: directory: %v, fileName: %v, %v", message.Directory, message.FileName, err)
		proc.errorKernel.errSend(proc, message, er, logWarning)

		return nil, err
	}
	defer f.Close()

	_, err = f.Write(message.Data)
	f.Sync()
	if err != nil {
		er := fmt.Errorf("error: methodEventTextLogging.handler: failed to write to file: file: %v, %v", file, err)
		proc.errorKernel.errSend(proc, message, er, logWarning)
	}

	ackMsg := []byte("confirmed from: " + node + ": " + fmt.Sprint(message.ID))
	return ackMsg, nil
}