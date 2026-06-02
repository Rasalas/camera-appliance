# Optional AgentDVR static stream setup

AgentDVR is not required for the normal camera-appliance workflow. If the optional Docker Compose profile is enabled for NVR experiments, configure AgentDVR manually to use only stable go2rtc aliases:

- `rtsp://go2rtc:8554/cam1`
- `rtsp://go2rtc:8554/cam2`
- `rtsp://go2rtc:8554/cam3`
- `rtsp://go2rtc:8554/cam4`
- `rtsp://go2rtc:8554/cam5`

Do not configure changing camera IP addresses in AgentDVR.
