# エンコーダー

エンコーダーはRabbitMQの`RABBITMQ_ENCODE_JOB_QUEUE`を購読し、動画をMPEG-DASHの単一品質へ変換します。入力ストリームを最初に`-c copy`でコンテナ変換し、入力がDASH互換でない場合だけ`libx264`とAACで再エンコードします。

ジョブは次のJSONです。

```json
{"video_id":"video-id","input_path":"/data/input/video.mp4","output_dir":"/data/output/video-id"}
```

処理中は`RABBITMQ_ENCODE_EVENT_QUEUE`へ`ENCODING`イベントを進捗率付きで発行し、成功時は`READY`と`manifest_path`、失敗時は`FAILED`と`error`を発行します。API側は`video_id`を所有者の動画へ対応付け、`ENCODING`、`READY`、`FAILED`の状態と進捗を永続化してください。イベントは次の形式です。

```json
{"video_id":"video-id","status":"READY","progress":1,"manifest_path":"/data/output/video-id/manifest.mpd","occurred_at":"2026-09-20T00:00:00Z"}
```

RabbitMQを使わない検証では`ENCODER_JOB_INPUT=stdin`を指定し、標準入力へ1行1ジョブでJSONを渡します。このモードは発行イベントを標準出力へJSON Linesで出力します。
