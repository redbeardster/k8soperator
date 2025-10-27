# Image URL to use all building/pushing image targets
IMG ?= redbeardster/pod-healer-operator:v1.0.0

# Build the docker image
docker-build:
	docker build -t ${IMG} .

# Push the docker image
docker-push:
	docker push ${IMG}

# Deploy to cluster
deploy:
	kubectl create namespace pod-healer-system --dry-run=client -o yaml | kubectl apply -f -
	kubectl apply -f manifests/rbac.yaml
	kubectl apply -f manifests/deployment.yaml

# Undeploy from cluster
undeploy:
	kubectl delete -f manifests/ --ignore-not-found=true
	kubectl delete namespace pod-healer-system --ignore-not-found=true

# Build and deploy
all: docker-build docker-push deploy
