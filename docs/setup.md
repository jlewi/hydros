# Installing Hydros On Your Repositories

1. Create a GitHub App
    * Grant it the permissions
        * Contents - Read & Write
        * Pull Requests - Read & Write
2. Generate a private key for the github app
3. Install it on the repositories that will be used as the source and destination
4. Download the latest hydros release from the [releases page](https://github.com/jlewi/hydros/releases)
5. Install the hydros binary on your system
6. Configure hydros to use the github app
 
   ```bash
   hydros config set github.appID=<YOUR GitHub App ID>
   hydros config set github.privateKey=/path/to/your/secret/key
   ```