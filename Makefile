# Plain ssh with the key gcloud generated: no Compute API round trip on every call.
VM_HOST ?= 34.135.96.121
VM_USER ?= $(USER)
IMAGE   := organizer
SSH     := ssh -i ~/.ssh/google_compute_engine $(VM_USER)@$(VM_HOST)

.PHONY: run test image deploy logs

run:
	set -a && . ./.env && set +a && go run ./cmd/bot

test:
	go test ./...

image:
	docker build --platform linux/amd64 -t $(IMAGE) .

# docker stop sends SIGTERM and waits, so the bot closes the database cleanly.
deploy: image
	docker save $(IMAGE) | gzip | $(SSH) 'gunzip | sudo docker load'
	$(SSH) 'sudo docker stop organizer; sudo docker rm organizer; \
		sudo docker run -d --name organizer --restart unless-stopped \
		--env-file /data/.env -v /data:/data $(IMAGE)'

logs:
	$(SSH) 'sudo docker logs -f --tail 100 organizer'

restart:
	$(SSH) 'sudo docker restart organizer'
