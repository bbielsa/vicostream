# vicostream

Stream h264 video from a VicoHome camera via WebRTC. Outputs raw h264 to stdout for piping to ffplay or ffmpeg.

## Build

```sh
make build
```

## Usage

```sh
export VICO_TOKEN="your-jwt-token"
export VICO_SN="your-camera-serial"

# Live playback
./vicostream | ffplay -f h264 -

# Record to MP4
./vicostream | ffmpeg -f h264 -i - -c copy output.mp4
```

Run `./vicostream --help` for more information.

## Clean

```sh
make clean
```
