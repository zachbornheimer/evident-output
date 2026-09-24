package mutationfacade

// run calls the mutation facade's methods from real call sites — the exact
// call sites the facade finding must enumerate.
func run() {
	c := CLI{Bin: "launchctl"}
	_ = c.Bootstrap("501", "/tmp/job.plist")
	_ = c.Up("/homelab/compose/docker-compose.yml", "qbittorrent")
	_ = c.WriteConfig("/tmp/config.json", []byte("{}"))
}
