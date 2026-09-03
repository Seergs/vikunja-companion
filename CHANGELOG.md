# Changelog

## [0.2.0](https://github.com/Seergs/vikunja-companion/compare/v0.1.0...v0.2.0) (2026-09-03)


### Features

* add internal package skeletons per design §7 ([8aae2a2](https://github.com/Seergs/vikunja-companion/commit/8aae2a2ef9ebb1fd21fd05307e2ae20c895297fa))
* add release please to the project ([7ec6eba](https://github.com/Seergs/vikunja-companion/commit/7ec6ebaa2a782aef8e32a3a3309802bf3c035573))
* add release please to the project ([64a3970](https://github.com/Seergs/vikunja-companion/commit/64a397095d913a873ae3303605b113092bc8b304))
* **companion:** GET/PUT /companion/v1/settings for digest prefs ([552bd5b](https://github.com/Seergs/vikunja-companion/commit/552bd5bd8df1a57678fab2289e7ac5436b27b64a))
* **companion:** identity cache, /companion/v1/info, and routing ([4cbcfae](https://github.com/Seergs/vikunja-companion/commit/4cbcfae4afd4a845622cc17abd3f33b89526a424))
* **companion:** log why an authenticated request was rejected ([ffcffd4](https://github.com/Seergs/vikunja-companion/commit/ffcffd48d63138299cbdc88efe817d8f4b145c4b))
* **companion:** minimal main with config validation and /healthz ([544ccea](https://github.com/Seergs/vikunja-companion/commit/544ccea4b39aa6f7824ebac2434b52fa7737c815))
* **companion:** start digest cron from main; COMPANION_DIGEST_ENABLED ([053166d](https://github.com/Seergs/vikunja-companion/commit/053166d3eb2484683440f38961952c23fa84129d))
* **companion:** webhook setup, inbound webhook, and device routes ([599eed6](https://github.com/Seergs/vikunja-companion/commit/599eed620b1f9b1758d21a68d4c147e6b72547f3))
* **companion:** wire dispatcher (relay + seal) and relay-token persistence into main ([9eb6a69](https://github.com/Seergs/vikunja-companion/commit/9eb6a6915cb3cb7e5a69e4a318b643690b290e12))
* **companion:** wire store, upstream probe, proxy, and router into main ([22128ac](https://github.com/Seergs/vikunja-companion/commit/22128ac65aaa742b178318f00497f8b77ffa58fa))
* **companion:** wire the Apprise sender, fall back to logging ([df1c7e8](https://github.com/Seergs/vikunja-companion/commit/df1c7e8cd7d4b209109af0a6c8f8131927a5453b))
* **config:** auto-load .env for local dev via godotenv ([e9ea561](https://github.com/Seergs/vikunja-companion/commit/e9ea561cc5a39eee7232d79e56a42933903a4472))
* **config:** load and validate companion and relay env config ([f90c7e4](https://github.com/Seergs/vikunja-companion/commit/f90c7e4383676f3822352e705eec20506ee677c1))
* **config:** read COMPANION_APPRISE_* env ([4a8bfb0](https://github.com/Seergs/vikunja-companion/commit/4a8bfb03c1bf8795089706168f758a848c1e9c22))
* **crypto:** sealed-box Seal and master-key Cipher for secrets at rest ([9d4493f](https://github.com/Seergs/vikunja-companion/commit/9d4493f02947ce2e6e0db98c9a14c7241d3d1bc7))
* **digest:** debug logs for skipped runs (disabled, off-window, already sent, empty) ([eca5928](https://github.com/Seergs/vikunja-companion/commit/eca59289e7abec6ba6a1a0058568cadec039fb6f))
* **digest:** friendlier briefing copy, mentions Vikunja ([e24605b](https://github.com/Seergs/vikunja-companion/commit/e24605b6970ae4904227830fc07c84394cd34723))
* **digest:** morning-briefing builder and 5-min cron runner ([0488a09](https://github.com/Seergs/vikunja-companion/commit/0488a09cc7f95a08e8ddf5dd129f2b62f0e10893))
* **notify:** add Level to Notification; overdue events are warnings ([c071e4a](https://github.com/Seergs/vikunja-companion/commit/c071e4ab78982e715c0c70560b87a9a32b12e171))
* **notify:** Apprise sender ([0c4e692](https://github.com/Seergs/vikunja-companion/commit/0c4e6920fc0858e73279b88eaf1597bda6bec975))
* **notify:** dedupe -&gt; seal -&gt; push dispatcher ([50c80f6](https://github.com/Seergs/vikunja-companion/commit/50c80f646e944a5918e0ff927edc2316a0cee9ae))
* **notify:** name the target URL in Apprise send errors ([e072d29](https://github.com/Seergs/vikunja-companion/commit/e072d29a5130c9ff07861df9888c012675e5dd2c))
* **proxy:** verbatim reverse proxy to the upstream Vikunja ([b043fe3](https://github.com/Seergs/vikunja-companion/commit/b043fe32a243c0e9f453b6b87b141519edcc7a48))
* **relay:** apns2-backed APNSSender with bad-token detection ([d853033](https://github.com/Seergs/vikunja-companion/commit/d8530339a6be5c11f17de4b3f47f3e8b66dc03e2))
* **relay:** companion-side client (register, push) ([58288ff](https://github.com/Seergs/vikunja-companion/commit/58288ff042c5b77e5e037ea6752a3721a8222e13))
* **relay:** minimal main with config validation and /healthz ([89c2130](https://github.com/Seergs/vikunja-companion/commit/89c213061fefaf64f194f2d7c6b6feeb548ee65c))
* **relay:** server (register, push, per-token rate limit, APNs envelope) ([73a51d8](https://github.com/Seergs/vikunja-companion/commit/73a51d872701759360293dd9f51a1941c672bc02))
* **relay:** wire cmd/relay to store, APNs, and the server ([59aa676](https://github.com/Seergs/vikunja-companion/commit/59aa6765528b7b593f9df6981817d2c183905176))
* **store:** create the database's parent directory if missing ([28665de](https://github.com/Seergs/vikunja-companion/commit/28665de7977c409092c94e3c14dce6ce4d2e09da))
* **store:** devices, webhooks-secret, and dedupe DAOs; reshape webhooks table ([f70664e](https://github.com/Seergs/vikunja-companion/commit/f70664ec4fde5275bb9e3875dc5b8da4ba17a666))
* **store:** relay token store (OpenRelay, MintToken, ValidToken) ([0aa6c50](https://github.com/Seergs/vikunja-companion/commit/0aa6c5064b985340a9b625587795b60b01469d8b))
* **store:** sqlite open, embedded migrations, and users DAO ([0bf7416](https://github.com/Seergs/vikunja-companion/commit/0bf74164b2e75c9e2a224d780a687a3e6cc3a465))
* **store:** user_settings table, digest DAOs, NotificationSent check ([d73384a](https://github.com/Seergs/vikunja-companion/commit/d73384a6d6e0a54602d5ff9292c1a1e58e0827b8))
* **vikunja:** GET /api/v1/tasks due-today fetch + user timezone ([9c1f3f8](https://github.com/Seergs/vikunja-companion/commit/9c1f3f882845bc14142b93585f50db8d993a5309))
* **vikunja:** thin client for /api/v1/info and /api/v1/user ([1dfcfa8](https://github.com/Seergs/vikunja-companion/commit/1dfcfa8a0127e7912500db2014868a85420ac315))
* **webhook:** HMAC signature verify and secret generation ([08e2398](https://github.com/Seergs/vikunja-companion/commit/08e2398e76aa28fc523fbac0b821c74fcf85fe56))
* **webhook:** map events to notifications (build) ([b101aee](https://github.com/Seergs/vikunja-companion/commit/b101aee4d21a86463a8f817600c5b3c0250f3cd1))
* **webhook:** parse the three event envelopes into typed values ([9fde33a](https://github.com/Seergs/vikunja-companion/commit/9fde33a3f646a9476dcf4fce8df754fcc17afd0b))


### Bug Fixes

* **notify:** keep relative deep links out of the Apprise body ([df57740](https://github.com/Seergs/vikunja-companion/commit/df577400723eba6db0d3df6288801b6e73385bf8))
* rename .renovate -&gt; renovate ([#5](https://github.com/Seergs/vikunja-companion/issues/5)) ([665b310](https://github.com/Seergs/vikunja-companion/commit/665b310b451e81874fdda687a566e465f508de6d))
* **renovate:** run go mod tidy after updating deps ([#13](https://github.com/Seergs/vikunja-companion/issues/13)) ([2e7d753](https://github.com/Seergs/vikunja-companion/commit/2e7d7537c8ba866e2bf8b13fb004145474ed2b81))
