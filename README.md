# Overview
`s3-sync` is a command line utility written in Go that syncs the contents of a local directory to an Amazon S3 bucket. It also has the ability to convert the modified remote files to a URL and write them to a file. This file can be used by other tools to purge edge caches. See [cf-purge](https://github.com/tonymackay/cf-purge) as an example of how to use the file to purge Cloudflare's edge cache.

## Building
Clone the repo then run the following commands:

```
go build
go install
```

To assign a version when building run:

```
go build -ldflags=-X=main.version=v1.0.0-beta1
```

## Using
The program requires the AWS CLI (v2.0+) to be installed on the machine it is running from and then the environment variables set as shown below:

```
export AWS_ACCESS_KEY_ID=<key_id> 
export AWS_SECRET_ACCESS_KEY=<key>
```

### Sync files
Sync the files located in the `www` directory to a bucket named `bucketname`.

```
s3-sync -source-dir www -bucket s3://bucketname
upload: www/hello-world/index.html to s3://testbucketforgo/hello-world/index.html
upload: www/hello-world/test-image.jpg to s3://testbucketforgo/hello-world/test-image.jpg
upload: www/index.html to s3://testbucketforgo/index.html
upload: www/sitemap.xml to s3://testbucketforgo/sitemap.xml
```

### Sync and log
Running the above command with the `-base-url` option will attempt to convert each remote file change to a URL and log them to a file (default urls.txt).

```
s3-sync -source-dir www -bucket s3://bucketname -base-url https://example.com
upload: www/hello-world/index.html to s3://testbucketforgo/hello-world/index.html
upload: www/hello-world/test-image.jpg to s3://testbucketforgo/hello-world/test-image.jpg
upload: www/index.html to s3://testbucketforgo/index.html
upload: www/sitemap.xml to s3://testbucketforgo/sitemap.xml
```

**Contents of urls.txt**

```
https://example.com/hello-world/
https://example.com/hello-world/test-image.jpg
https://example.com/index.html
https://example.com/sitemap.xml
```

## License
[MIT License](LICENSE)
