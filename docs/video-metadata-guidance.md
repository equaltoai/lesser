Essential vs Nice-to-Have Metadata

  Essential Metadata (Required for Streaming)

  type EssentialVideoMetadata struct {
      // Critical for playback
      Core: CoreMetadata{
          Duration:      time.Duration `json:"duration" 
  required:"true"`
          Width:         int          `json:"width" 
  required:"true"`
          Height:        int          `json:"height" 
  required:"true"`
          Bitrate:       int64        `json:"bitrate" 
  required:"true"`
          Codec:         string       `json:"codec" 
  required:"true"`        // h264, h265, av1
          Container:     string       `json:"container" 
  required:"true"`     // mp4, webm, mkv
          FrameRate:     float64      `json:"frame_rate" 
  required:"true"`
          HasAudio:      bool         `json:"has_audio" 
  required:"true"`
      },

      // Required for adaptive streaming
      Streaming: StreamingMetadata{
          KeyframeInterval: int
  `json:"keyframe_interval"` // GOP size
          CanSeek:         bool           `json:"can_seek"`
          IsProgressive:   bool           `json:"is_progressive"`
     // Progressive download capable
          Profiles: []StreamingProfile{
              {Resolution: "1080p", Bitrate: 5000000, Codec:
  "h264"},
              {Resolution: "720p",  Bitrate: 2500000, Codec:
  "h264"},
              {Resolution: "480p",  Bitrate: 1000000, Codec:
  "h264"},
          },
      },

      // Audio track info (if present)
      Audio: AudioMetadata{
          Codec:       string  `json:"audio_codec"`      // aac, 
  opus, mp3
          Channels:    int     `json:"audio_channels"`
          SampleRate:  int     `json:"sample_rate"`
          Bitrate:     int     `json:"audio_bitrate"`
      },

      // File integrity
      Integrity: IntegrityMetadata{
          FileSize:    int64   `json:"file_size" required:"true"`
          MD5Hash:     string  `json:"md5_hash"`
          IsComplete:  bool    `json:"is_complete" required:"true"`
          IsPlayable:  bool    `json:"is_playable" required:"true"`
      },
  }

  Nice-to-Have Metadata (Enhanced Experience)

  type EnhancedVideoMetadata struct {
      // Visual enhancements
      Visual: VisualMetadata{
          AspectRatio:     string   `json:"aspect_ratio"`      // 
  16:9, 4:3, etc
          ColorSpace:      string   `json:"color_space"`       // 
  rec709, rec2020
          HDR:             bool     `json:"hdr"`
          DynamicRange:    string   `json:"dynamic_range"`     // 
  SDR, HDR10, Dolby Vision
          PixelFormat:     string   `json:"pixel_format"`      // 
  yuv420p, yuv422p
          BitDepth:        int      `json:"bit_depth"`         // 
  8, 10, 12
      },

      // Content description
      Content: ContentMetadata{
          Title:           string   `json:"title"`
          Description:     string   `json:"description"`
          Language:        string   `json:"language"`
          Thumbnail:       string   `json:"thumbnail_url"`
          PreviewClip:     string   `json:"preview_clip_url"`
          Chapters:        []Chapter `json:"chapters"`
          Subtitles:       []Subtitle `json:"subtitles"`
      },

      // Advanced streaming
      Advanced: AdvancedMetadata{
          SceneChanges:    []int64  `json:"scene_changes"`     // 
  Timestamps for smart cutting
          MotionVectors:   []byte   `json:"motion_vectors"`    // 
  For efficient encoding
          Complexity:      float64  `json:"complexity_score"`  // 
  Encoding complexity
          OptimalBitrates: map[string]int64
  `json:"optimal_bitrates"`
      },

      // Analytics hints
      Analytics: AnalyticsMetadata{
          PeakBitrate:     int64    `json:"peak_bitrate"`
          AverageBitrate:  int64    `json:"average_bitrate"`
          BitrateVariance: float64  `json:"bitrate_variance"`
          QualityScore:    float64  `json:"quality_score"`     // 
  VMAF/PSNR
          BufferHealth:    float64  `json:"buffer_health"`
      },
  }

  Handling Corrupted/Non-Standard Files

  Multi-Stage Validation Pipeline

  type VideoValidationPipeline struct {
      stages: []ValidationStage{
          {Name: "QuickCheck",     Timeout: 1 * time.Second},
          {Name: "DeepAnalysis",   Timeout: 10 * time.Second},
          {Name: "Repair",         Timeout: 30 * time.Second},
          {Name: "Transcode",      Timeout: 5 * time.Minute},
      },
  }

  func (p *VideoValidationPipeline) Process(videoPath string) 
  (*VideoMetadata, error) {
      var lastError error

      for _, stage := range p.stages {
          ctx, cancel := context.WithTimeout(context.Background(),
  stage.Timeout)
          defer cancel()

          result, err := p.runStage(ctx, stage.Name, videoPath)
          if err == nil {
              return result, nil
          }

          lastError = err
          p.logger.Warn("Stage failed, trying next",
              zap.String("stage", stage.Name),
              zap.Error(err))

          // Continue to next stage for recovery attempt
      }

      return nil, fmt.Errorf("all validation stages failed: %w",
  lastError)
  }

  Stage 1: Quick Check (Fast Fail)

  func (p *VideoValidationPipeline) quickCheck(ctx context.Context,
   path string) error {
      // Use ffprobe with minimal parsing
      cmd := exec.CommandContext(ctx, "ffprobe",
          "-v", "quiet",
          "-print_format", "json",
          "-show_format",
          "-show_streams",
          "-read_intervals", "%+1", // Read only first second
          path,
      )

      output, err := cmd.Output()
      if err != nil {
          return &ValidationError{
              Stage:  "QuickCheck",
              Reason: "Cannot read file headers",
              Fatal:  true,
          }
      }

      // Check for minimum required fields
      var probe FFProbeResult
      if err := json.Unmarshal(output, &probe); err != nil {
          return &ValidationError{
              Stage:  "QuickCheck",
              Reason: "Invalid metadata structure",
              Fatal:  false, // Try deep analysis
          }
      }

      // Validate essentials
      if probe.Format.Duration == 0 {
          return ErrNoDuration
      }
      if len(probe.Streams) == 0 {
          return ErrNoStreams
      }

      return nil
  }

  Stage 2: Deep Analysis (Thorough Check)

  func (p *VideoValidationPipeline) deepAnalysis(ctx 
  context.Context, path string) (*VideoMetadata, error) {
      // Full file scan with error detection
      cmd := exec.CommandContext(ctx, "ffmpeg",
          "-v", "error",           // Only show errors
          "-i", path,
          "-f", "null",            // Null output
          "-max_error_rate", "0.01", // 1% error threshold
          "-xerror",               // Exit on error
          "-",
      )

      var stderr bytes.Buffer
      cmd.Stderr = &stderr

      err := cmd.Run()
      errorOutput := stderr.String()

      if err != nil {
          // Parse specific errors
          switch {
          case strings.Contains(errorOutput, "moov atom not 
  found"):
              return nil, &RepairableError{
                  Type:   "MISSING_MOOV",
                  Action: "REPAIR_MOOV",
              }

          case strings.Contains(errorOutput, "Invalid NAL unit"):
              return nil, &RepairableError{
                  Type:   "CORRUPT_NAL",
                  Action: "REMUX",
              }

          case strings.Contains(errorOutput, "non monotonically 
  increasing"):
              return nil, &RepairableError{
                  Type:   "TIMESTAMP_ERROR",
                  Action: "FIX_TIMESTAMPS",
              }

          default:
              return nil, &UnrepairableError{
                  Message: errorOutput,
              }
          }
      }

      // Extract detailed metadata
      return p.extractFullMetadata(path)
  }

  Stage 3: Repair Attempts

  type VideoRepairer struct {
      strategies: map[string]RepairStrategy{
          "MISSING_MOOV": {
              Name: "Recover MOOV atom",
              Func: repairMoovAtom,
          },
          "CORRUPT_NAL": {
              Name: "Remux container",
              Func: remuxContainer,
          },
          "FIX_TIMESTAMPS": {
              Name: "Rebuild timestamps",
              Func: rebuildTimestamps,
          },
          "BROKEN_INDEX": {
              Name: "Rebuild index",
              Func: rebuildIndex,
          },
      },
  }

  func (r *VideoRepairer) Repair(path string, errorType string) 
  (string, error) {
      strategy, exists := r.strategies[errorType]
      if !exists {
          return "", ErrNoRepairStrategy
      }

      // Create temporary file for repaired version
      tempPath := fmt.Sprintf("%s.repaired.mp4", path)

      // Attempt repair
      if err := strategy.Func(path, tempPath); err != nil {
          return "", fmt.Errorf("repair failed: %w", err)
      }

      // Validate repaired file
      if err := r.validateRepaired(tempPath); err != nil {
          os.Remove(tempPath)
          return "", fmt.Errorf("repaired file invalid: %w", err)
      }

      return tempPath, nil
  }

  // Example repair function
  func remuxContainer(input, output string) error {
      // Use ffmpeg to copy streams to new container
      cmd := exec.Command("ffmpeg",
          "-i", input,
          "-c", "copy",           // Copy codecs, don't re-encode
          "-movflags", "faststart", // Optimize for streaming
          "-y",                    // Overwrite
          output,
      )

      return cmd.Run()
  }

  Stage 4: Fallback Transcoding

  type FallbackTranscoder struct {
      // Progressive quality degradation
      profiles: []TranscodeProfile{
          {
              Name:   "HighQuality",
              Preset: "slow",
              CRF:    23,
          },
          {
              Name:   "Standard",
              Preset: "medium",
              CRF:    28,
          },
          {
              Name:   "FastFallback",
              Preset: "ultrafast",
              CRF:    35,
          },
      },
  }

  func (t *FallbackTranscoder) TranscodeWithFallback(input string) 
  (string, error) {
      for _, profile := range t.profiles {
          output := fmt.Sprintf("%s.%s.mp4", input, profile.Name)

          cmd := exec.Command("ffmpeg",
              "-i", input,
              "-c:v", "libx264",
              "-preset", profile.Preset,
              "-crf", fmt.Sprintf("%d", profile.CRF),
              "-c:a", "aac",
              "-b:a", "128k",
              "-movflags", "+faststart",
              "-max_muxing_queue_size", "1024",
              "-y",
              output,
          )

          if err := cmd.Run(); err != nil {
              t.logger.Warn("Transcode profile failed",
                  zap.String("profile", profile.Name),
                  zap.Error(err))
              continue
          }

          // Validate output
          if err := t.validate(output); err == nil {
              return output, nil
          }
      }

      return "", ErrAllTranscodesFailed
  }

  Error Recovery Strategies

  Graceful Degradation

  type DegradationStrategy struct {
      Handle: func(video *VideoFile, error error) 
  (*StreamingOptions, error) {
          switch error.(type) {
          case *CorruptVideoError:
              // Offer lower quality or audio-only
              return &StreamingOptions{
                  Mode:     "AUDIO_ONLY",
                  Message:  "Video corrupted, audio available",
                  Fallback: video.AudioTrackURL,
              }, nil

          case *UnsupportedCodecError:
              // Offer download instead of streaming
              return &StreamingOptions{
                  Mode:     "DOWNLOAD_ONLY",
                  Message:  "Format not streamable, download 
  available",
                  Fallback: video.DownloadURL,
              }, nil

          case *PartialFileError:
              // Stream what's available
              return &StreamingOptions{
                  Mode:     "PARTIAL_STREAM",
                  Message:  "Partial content available",
                  Fallback: video.PartialURL,
                  Range:    video.AvailableRange,
              }, nil

          default:
              return nil, error
          }
      },
  }

  Metadata Extraction Fallbacks

  func ExtractMetadataWithFallbacks(path string) (*VideoMetadata, 
  error) {
      // Try multiple tools in order of preference
      extractors := []MetadataExtractor{
          &FFProbeExtractor{},     // Most reliable
          &MediaInfoExtractor{},   // More format support
          &ExifToolExtractor{},    // Basic metadata
          &FileHeaderParser{},     // Last resort
      }

      var lastError error
      for _, extractor := range extractors {
          metadata, err := extractor.Extract(path)
          if err == nil {
              return metadata, nil
          }
          lastError = err
      }

      // Return partial metadata if possible
      return &VideoMetadata{
          FileSize:   getFileSize(path),
          IsPlayable: false,
          Error:      lastError.Error(),
      }, lastError
  }

  Recommended Production Configuration

  config := &VideoProcessingConfig{
      // Metadata requirements
      RequiredFields: []string{
          "duration", "width", "height",
          "codec", "bitrate", "frame_rate",
      },

      // Validation thresholds
      MaxProcessingTime:    30 * time.Second,
      MaxFileSize:         5 * 1024 * 1024 * 1024, // 5GB
      MinDuration:         100 * time.Millisecond,
      MaxErrorRate:        0.01, // 1% frame errors

      // Repair settings
      EnableAutoRepair:    true,
      MaxRepairAttempts:   3,
      RepairTimeout:       1 * time.Minute,

      // Fallback options
      EnableTranscoding:   true,
      TranscodeTimeout:    5 * time.Minute,
      KeepOriginal:       true,

      // Caching
      CacheMetadata:      true,
      MetadataTTL:        24 * time.Hour,

      // Error handling
      QuarantineCorrupt:  true,
      NotifyOnFailure:    true,
      RetryAfter:         1 * time.Hour,
  }