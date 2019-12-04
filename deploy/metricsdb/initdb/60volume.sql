/* Copyright 2019 Tad Lebeck
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *    http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

\c nuvo_metrics

-- V1 Volume metrics table with highly temporally variable data
-- It references the more stable VolumeMetadata table
CREATE TABLE IF NOT EXISTS VolumeMetrics (
    Timestamp timestamptz NOT NULL,
    VolumeMetadataNum integer NOT NULL REFERENCES VolumeMetadata MATCH FULL ON DELETE CASCADE,
    -- metrics and analysis for this datum
    BytesRead bigint,
    BytesWritten bigint,
    NumberReads integer,
    NumberWrites integer,
    LatencyMean integer, -- Unit: Microseconds
    LatencyMax integer,  -- Unit: Microseconds
    SampleDuration smallint, -- Unit: seconds
    ViolationLatencyMean integer, -- non-zero if above compliance mean
    ViolationLatencyMax integer, -- number of times above compliance max
    ViolationWorkloadRate bigint, -- quantity (IOPs or Bytes) above compliance limit
    ViolationWorkloadMixRead bigint, -- quantity (IOPS or Bytes) of reads above compliance max limit
    ViolationWorkloadMixWrite bigint, -- quantity (IOPS or Bytes) of writes above compliance max limit
    ViolationWorkloadAvgSizeMin bigint, -- quantity (Bytes) below compliance limit
    ViolationWorkloadAvgSizeMax bigint, -- quantity (Bytes) above compliance limit
    NumCacheReadUserHits integer,
    NumCacheReadUserTotal integer,
    NumCacheReadMetaHits integer,
    NumCacheReadMetaTotal integer
);
CREATE UNIQUE INDEX IF NOT EXISTS VolumeMetricsI1 ON VolumeMetrics (Timestamp DESC, VolumeMetadataNum);
INSERT INTO SchemaVersion (TableName, Version) VALUES ('VolumeMetrics', 1) ON CONFLICT DO NOTHING;

-- Convert VolumeMetrics to a hypertable (note: need lowercase)
SELECT create_hypertable (
    'volumemetrics',
    'timestamp',
    chunk_time_interval => interval '7 day',
    create_default_indexes => FALSE,
    if_not_exists => TRUE);

-- V1 denormalized insertion helper
CREATE OR REPLACE FUNCTION VolumeMetricsInsert1(
    pT timestamptz, pVolumeID uuid,
    pBytesRead bigint, pBytesWritten bigint,
    pNumberReads integer, pNumberWrites integer,
    pLatencyMean integer, pLatencyMax integer,
    pSampleDuration smallint,
    pViolationLatencyMean integer,
    pViolationLatencyMax integer,
    pViolationWorkloadRate bigint,
    pViolationWorkloadMixRead bigint,
    pViolationWorkloadMixWrite bigint,
    pViolationWorkloadAvgSizeMin bigint,
    pViolationWorkloadAvgSizeMax bigint,
    pNumCacheReadUserHits integer, pNumCacheReadUserTotal integer,
    pNumCacheReadMetaHits integer, pNumCacheReadMetaTotal integer
) RETURNS void
AS $$
DECLARE
    vmNum integer := (
        SELECT max(VolumeMetadataNum)
        FROM VolumeMetadata VM JOIN VolumeObjects V ON VM.VolumeNum = V.VolumeNum
        WHERE V.VolumeID = pVolumeID AND VM.Timestamp <= pT);
BEGIN
    INSERT INTO VolumeMetrics(
            Timestamp, VolumeMetadataNum,
            BytesRead, BytesWritten,
            NumberReads, NumberWrites,
            LatencyMean, LatencyMax,
            SampleDuration,
            ViolationLatencyMean,
            ViolationLatencyMax,
            ViolationWorkloadRate,
            ViolationWorkloadMixRead,
            ViolationWorkloadMixWrite,
            ViolationWorkloadAvgSizeMin,
            ViolationWorkloadAvgSizeMax,
            NumCacheReadUserHits,
            NumCacheReadUserTotal,
            NumCacheReadMetaHits,
            NumCacheReadMetaTotal
        ) VALUES (
            pT, vmNum,
            pBytesRead, pBytesWritten,
            pNumberReads, pNumberWrites,
            pLatencyMean, pLatencyMax,
            pSampleDuration,
            pViolationLatencyMean,
            pViolationLatencyMax,
            pViolationWorkloadRate,
            pViolationWorkloadMixRead,
            pViolationWorkloadMixWrite,
            pViolationWorkloadAvgSizeMin,
            pViolationWorkloadAvgSizeMax,
            pNumCacheReadUserHits,
            pNumCacheReadUserTotal,
            pNumCacheReadMetaHits,
            pNumCacheReadMetaTotal
        ) ON CONFLICT DO NOTHING;
END
$$
LANGUAGE plpgsql;
