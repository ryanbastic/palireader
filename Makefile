build:
	docker build --no-cache -t palireader .

push:
	docker tag palireader rbastic/palireader:release
	docker push rbastic/palireader:release
