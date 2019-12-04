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

-- V1 Pool metrics table
CREATE TABLE IF NOT EXISTS PoolMetrics(
    Timestamp timestamptz NOT NULL,
    PoolNum integer REFERENCES PoolObjects MATCH FULL ON DELETE CASCADE,
    -- contextual columns
    DomainNum integer REFERENCES DomainObjects MATCH FULL ON DELETE CASCADE,
    -- metrics for this datum
    TotalBytes bigint,
    AvailableBytes bigint,
    ReservableBytes bigint
);
CREATE UNIQUE INDEX IF NOT EXISTS PoolMetricsI1 ON PoolMetrics(Timestamp DESC, PoolNum);
INSERT INTO SchemaVersion(TableName, Version) VALUES ('PoolMetrics', 1) ON CONFLICT DO NOTHING;

-- Convert PoolMetrics to a hypertable (note: need lowercase)
SELECT create_hypertable('poolmetrics', 'timestamp',
    chunk_time_interval => interval '7 day',
    create_default_indexes => FALSE,
    if_not_exists => TRUE);

-- V1 denormalized insertion helper
CREATE OR REPLACE FUNCTION PoolMetricsInsert1(
    pT timestamptz, pPoolID uuid,
    pDomID uuid,
    pTotalBytes bigint, pAvailableBytes bigint, pReservableBytes bigint
) RETURNS void
AS $$
BEGIN
    INSERT INTO PoolMetrics(
            Timestamp, PoolNum,
            DomainNum,
            TotalBytes, AvailableBytes, ReservableBytes
        ) VALUES (
            pT, PoolNumber(pPoolID),
            DomainNumber(pDomID),
            pTotalBytes, pAvailableBytes, pReservableBytes
        ) ON CONFLICT DO NOTHING;
END
$$
LANGUAGE plpgsql;

CREATE OR REPLACE VIEW PoolMetrics1 AS
SELECT PM.Timestamp, PM.PoolNum, PM.DomainNum, PM.TotalBytes, PM.AvailableBytes, PM.ReservableBytes,
    PO.PoolID, D.DomainID
FROM PoolMetrics PM
JOIN PoolObjects PO ON PM.PoolNum = PO.PoolNum
JOIN DomainObjects D ON PM.DomainNum = D.DomainNum
;
