# Change Log
All notable changes to this project will be documented in this file based on the
[Keep a Changelog](http://keepachangelog.com/) Standard.

## [Unreleased](https://github.com/elastic/libbeat/compare/1.0.0-beta4...HEAD)

### Backward Compatibility Breaks

### Bugfixes
- Use stderr for console log output. #219
- Handle empty event array in publisher. #207
- Limit number of workers for Elasticsearch output to 1 per configured host. packetbeat#226
- Respect '*' debug selector in IsDebug. #226 (elastic/packetbeat#339)
- Limit number of workers for Elasticsearch output. elastic/packetbeat#226
- On Windows, remove service related error message when running in the console. #242
- Fix waitRetry no configured in single output mode configuration. elastic/filebeat#144

### Added
- Add Console output plugin. #218
- Rename the timestamp field with @timestamp. #237

### Improvements
- Send @metadata.beat to Logstash instead of @metadata.index to prevent
  possible name clashes and give user full control over index name used for
  Elasticsearch
- Add logging messages for bulk publishing in case of error #229
- Add option to configure number of parallel workers publishing to Elasticsearch
  or Logstash.

### Deprecated


## [1.0.0-beta4](https://github.com/elastic/libbeat/compare/1.0.0-beta3...1.0.0-beta4)

### Backward Compatibility Breaks
- Update tls config options naming from dash to underline #162
- Feature/output modes: Introduction of PublishEvent(s) to be used by beats #118 #115

### Bugfixes
- Determine Elasticsearch index for an event based on UTC time #81
- Fixing ES output's defaultDeadTimeout so that it is 60 seconds #103
- Es outputer: fix timestamp conversion #91
- Fix TLS insecure config option #239

### Added
- Add logstash output plugin #151
- Integration tests for Beat -> Logstash -> Elasticsearch added #195 #188 #168 #137 #128 #112
- Large updates and improvements to the documentation
- Add direction field to publisher output to indicate inbound/outbound transactions #150

### Improvements
- Add tls configuration support to elasticsearch and logstash outputers #139
- All external dependencies were updated to the latest version. Update to Golang 1.5.1 #162
- Guarantee ES index is based in UTC time zone #164
- Cache: optional per element timeout #144
- Make it possible to set hosts in different ways. #135
- Expose more TLS config options #124
- Use the Beat name in the default configuration file path #99

### Deprecated
- Redis output was deprecated #169 #145
- Host and port configuration options are deprecated. They are replaced by the hosts
 configuration option. #141
