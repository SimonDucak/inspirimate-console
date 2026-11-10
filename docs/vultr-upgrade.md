# Upgrade the Vultr instance

Run this on the server as root. It updates the code in `/opt/sandboxd` and
restarts the stack. The SQLite database stays where it is. `upgrade.sh` copies
`sandboxd.db` and `.env` to a backup before it changes anything, and restores
that copy if the new build does not come up healthy.

Git credentials, model connections, apps, and workspaces live in that database.
Do not copy a database from another machine over it.

## Every upgrade

```bash
cd /opt/sandboxd
git remote set-url origin https://github.com/SimonDucak/inspirimate-console.git
bash upgrade.sh --yes
```

`--yes` only skips the release-notes question. The database backup still happens.

When it finishes, refresh the console in the browser.

## Flutter

New sandboxes use whatever image `SANDBOXD_IMAGE` names. After an upgrade, check
that Flutter is still in `sandboxd-flutter:0.3.0`:

```bash
cd /opt/sandboxd
docker run --rm --entrypoint test sandboxd-flutter:0.3.0 -x /opt/flutter/bin/flutter && echo flutter-ok
```

`flutter-ok` means the image is fine. Existing sandboxes keep the image they
were created with. New ones pick up this image.

If that command does not print `flutter-ok`, rebuild Flutter before creating a
new sandbox. The build downloads the Flutter SDK and takes several minutes.

```bash
cd /opt/sandboxd
docker tag sandboxd-flutter:0.3.0 sandboxd-base:0.3.0
docker build -t sandboxd-flutter:0.3.0 -f image/flutter/Dockerfile image/flutter
docker run --rm --entrypoint test sandboxd-flutter:0.3.0 -x /opt/flutter/bin/flutter && echo flutter-ok
```

If the tag line says the image does not exist, skip it and run the `docker build`
line. That builds Flutter on top of `sandboxd-base:0.3.0`.

Confirm the instance is set to use that image:

```bash
grep '^SANDBOXD_IMAGE=' /opt/sandboxd/.env
```

It should be `SANDBOXD_IMAGE=sandboxd-flutter:0.3.0`. If it is empty or points at
`sandboxd-base`, set it and recreate the control plane. This restarts the stack
and leaves the database in place.

```bash
cd /opt/sandboxd
grep -q '^SANDBOXD_IMAGE=' .env && sed -i 's#^SANDBOXD_IMAGE=.*#SANDBOXD_IMAGE=sandboxd-flutter:0.3.0#' .env || printf 'SANDBOXD_IMAGE=sandboxd-flutter:0.3.0\n' >> .env
docker compose --profile console up -d
```

## Android APK builds

The Flutter image also carries the Android SDK (JDK 21, platforms 34 and 35, build-tools 34 and 35, the NDKs this Flutter release and its plugins expect, and CMake 3.22.1). After pulling the code that adds or changes those packages, rebuild the image. The download is large and takes longer than the Flutter-only build.

```bash
cd /opt/sandboxd
docker build -t sandboxd-flutter:0.3.0 -f image/flutter/Dockerfile image/flutter
```

New sandboxes use that image. An existing sandbox keeps the image it was created with until it is recreated, so it cannot build an APK until then.

On a 4 GB machine, run one APK build at a time and stop other sandboxes first. The build is capped to a 1 GB Gradle heap, and it is slow.
