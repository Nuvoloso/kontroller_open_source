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

-- V1 SPA metrics table
CREATE TABLE IF NOT EXISTS SPAMetrics(
    Timestamp timestamptz NOT NULL,
    SPANum integer REFERENCES SPAObjects MATCH FULL ON DELETE CASCADE,
    -- contextual columns
    DomainNum integer REFERENCES DomainObjects MATCH FULL ON DELETE CASCADE,
    ClusterNum integer REFERENCES ClusterObjects MATCH FULL ON DELETE CASCADE,
    ServicePlanNum integer REFERENCES ServicePlanObjects MATCH FULL ON DELETE CASCADE,
    AccountNum integer REFERENCES AccountObjects MATCH FULL ON DELETE CASCADE,
    -- metrics for this datum
    TotalBytes bigint,
    ReservableBytes bigint
);
CREATE UNIQUE INDEX IF NOT EXISTS SPAMetricsI1 ON SPAMetrics(Timestamp DESC, SPANum);
INSERT INTO SchemaVersion(TableName, Version) VALUES ('SPAMetrics', 1) ON CONFLICT DO NOTHING;

-- Convert SPAMetrics to a hypertable (note: need lowercase)
SELECT create_hypertable('spametrics', 'timestamp',
    chunk_time_interval => interval '7 day',
    create_default_indexes => FALSE,
    if_not_exists => TRUE);

-- V1 denormalized insertion helper
CREATE OR REPLACE FUNCTION SPAMetricsInsert1(
    pT timestamptz,
    pSPAID uuid,
    pDomID uuid,
    pClusterID uuid,
    pServicePlanID uuid,
    pAccountID uuid,
    pTotalBytes bigint,
    pReservableBytes bigint
) RETURNS void
AS $$
BEGIN
    INSERT INTO SPAMetrics(
            Timestamp, SPANum,
            DomainNum, ClusterNum, ServicePlanNum, AccountNum,
            TotalBytes, ReservableBytes
        ) VALUES (
            pT, SPANumber(pSPAID),
            DomainNumber(pDomID), ClusterNumber(pClusterID), ServicePlanNumber(pServicePlanID), AccountNumber(pAccountID),
            pTotalBytes, pReservableBytes
        ) ON CONFLICT DO NOTHING;
END
$$
LANGUAGE plpgsql;

CREATE OR REPLACE VIEW SPAMetrics1 AS
SELECT SPAM.Timestamp, SPAM.SPANum, SPAM.DomainNum, SPAM.ClusterNum, SPAM.ServicePlanNum, SPAM.AccountNum, SPAM.TotalBytes, SPAM.ReservableBytes,
    SPAO.SPAID, D.DomainID, CO.ClusterID, SPO.ServicePlanId, AO.AccountID
FROM SPAMetrics SPAM
JOIN SPAObjects SPAO ON SPAM.SPANum = SPAO.SPANum
JOIN DomainObjects D ON SPAM.DomainNum = D.DomainNum
JOIN ClusterObjects CO ON SPAM.ClusterNum = CO.ClusterNum
JOIN ServicePlanObjects SPO ON SPAM.ServicePlanNum = SPO.ServicePlanNum
JOIN AccountObjects AO ON SPAM.AccountNum = AO.AccountNum
;
