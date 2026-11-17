//go:build !windows

package updater

const mainUnit = `# Managed by Loom native installer
[Unit]
Description=Loom media automation
Wants=network-online.target
After=network-online.target
StartLimitIntervalSec=60
StartLimitBurst=5

[Service]
Type=simple
User=loom
Group=loom
WorkingDirectory=/var/lib/loom
Environment=HOME=/var/lib/loom
ExecStart=/usr/local/lib/loom/loom serve --config /etc/loom/loom.yaml
Restart=on-failure
RestartSec=10
TimeoutStopSec=120
KillSignal=SIGTERM
UMask=0027
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=full
ReadWritePaths=/etc/loom /var/lib/loom
StandardOutput=journal
StandardError=journal
SyslogIdentifier=loom

[Install]
WantedBy=multi-user.target
`

const updaterUnit = `# Managed by Loom native updater installer
[Unit]
Description=Loom signed native update
Wants=network-online.target
After=network-online.target

[Service]
Type=oneshot
User=root
Group=root
ExecStart=/usr/local/lib/loom/loom-updater
TimeoutStartSec=20min
TimeoutStopSec=5min
UMask=0077
NoNewPrivileges=true
PrivateTmp=true
ProtectHome=true
ProtectSystem=strict
ReadWritePaths=/usr/local/lib/loom /etc/loom /var/lib/loom /var/lib/loom-updater
`

const recoverUnit = `# Managed by Loom native updater installer
[Unit]
Description=Recover an interrupted Loom native update
Before=loom.service

[Service]
Type=oneshot
User=root
Group=root
ExecStart=/usr/local/lib/loom/loom-updater recover
TimeoutStartSec=5min
UMask=0077
NoNewPrivileges=true
PrivateTmp=true
ProtectHome=true
ProtectSystem=strict
ReadWritePaths=/usr/local/lib/loom /etc/loom /var/lib/loom /var/lib/loom-updater
`

const timerUnit = `# Managed by Loom native updater installer
[Unit]
Description=Check opted-in Loom native updates hourly

[Timer]
OnCalendar=hourly
RandomizedDelaySec=15min
Persistent=true
Unit=loom-updater.service

[Install]
WantedBy=timers.target
`

const dependencyUnit = `# Managed by Loom native updater installer
[Unit]
Requires=loom-updater-recover.service
After=loom-updater-recover.service
`
