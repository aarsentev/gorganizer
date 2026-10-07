# Plain ssh with the key gcloud generated: no Compute API round trip on every call.
VM_HOST ?= 34.135.96.121
VM_USER ?= $(USER)
IMAGE   := organizer
SSH     := ssh -i ~/.ssh/google_compute_engine $(VM_USER)@$(VM_HOST)

.PHONY: run test image deploy upload logs restart reboot docker-start docker-stop

run:
	set -a && . ./.env && set +a && go run ./cmd/bot

test:
	go test ./...

image: docker-start
	docker build --platform linux/amd64 -t $(IMAGE) .

# Docker Desktop is needed only for the build. deploy quits it again,
# but only if it was not running before: other projects may be using it.
deploy:
	@if docker info >/dev/null 2>&1; then \
		$(MAKE) upload; \
	else \
		$(MAKE) upload; status=$$?; $(MAKE) docker-stop; exit $$status; \
	fi

# docker stop sends SIGTERM and waits, so the bot closes the database cleanly.
upload: image
	docker save $(IMAGE) | gzip | $(SSH) 'gunzip | sudo docker load'
	$(SSH) 'sudo docker stop organizer; sudo docker rm organizer; \
		sudo docker run -d --name organizer --restart unless-stopped \
		--env-file /data/.env -v /data:/data $(IMAGE)'

logs:
	$(SSH) 'sudo docker logs -f --tail 100 organizer'

restart:
	$(SSH) 'sudo docker restart organizer'

reboot:
	$(SSH) 'sudo reboot'

# docker desktop start returns once the engine is up.
docker-start:
	@docker info >/dev/null 2>&1 || docker desktop start --timeout 120

docker-stop:
	docker desktop stop
