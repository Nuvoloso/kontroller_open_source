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
-- THROWAWAY SUPPORT CODE TO PLAY WITH THE SCHEMA
-- NOT FOR PRODUCTION

\c nuvo_metrics

-- The idof() function is provided to generate a consistent fake uuid for a given string.
-- Use it wherever uuids are required.

-- Experimenting with volume metrics:
--
--   SELECT generate_vm1(current_timestamp - interval '30days', '30 day', 'volume1', 'account1', 'sp1', 'c1', 10000000, 0, 1000000, 'cg1', ARRAY['AG1']);
--
-- will generate fake data for 30 days, starting 30 days ago.
-- To generate fake data for multiple volumes it is easier to specify an absolute time:
--
-- SELECT generate_vm1('2018-01-01 00:00:00.0+00'::timestamptz, '90 day', 'volume1', 'account1', 'sp1', 'c1', 10000000,   0, 1000000, 500000, 500000, 'cg1', ARRAY['AG1']);
-- SELECT generate_vm1('2018-04-01 00:00:00.0+00'::timestamptz, '30 day', 'volume1', 'account1', 'sp1', 'c1', 10000000, 100, 3000100, 500000, 250000, 'cg1', ARRAY['AG1', 'AG2']);
-- SELECT generate_vm1('2018-01-01 00:00:00.0+00'::timestamptz, '90 day', 'volume2', 'account1', 'sp1', 'c1', 10000000, 200, 1000200, 500000, 250000, 'cg1', ARRAY['AG2']);
-- SELECT generate_vm1('2018-04-01 00:00:00.0+00'::timestamptz, '30 day', 'volume2', 'account1', 'sp3', 'c1', 10000000,   0, 1000000, 500000, 500000, 'cg1', ARRAY['AG2']);
-- SELECT generate_vm1('2018-01-01 00:00:00.0+00'::timestamptz, '90 day', 'volume3', 'account2', 'sp1', 'c1',  1000000,   1, 1000001, 500000, 500000, 'cg2', ARRAY['AG3']);
-- SELECT generate_vm1('2018-04-01 00:00:00.0+00'::timestamptz, '30 day', 'volume3', 'account2', 'sp2', 'c1',  1000000,   2, 1000002, 500000, 500000, 'cg2', ARRAY['AG3', 'AG4']);
-- SELECT generate_vm1('2018-01-01 00:00:00.0+00'::timestamptz, '90 day', 'volume4', 'account2', 'sp2', 'c1',  1000000,   3, 1000003, 500000, 500000, 'cg2', ARRAY['AG4']);
-- SELECT generate_vm1('2018-04-01 00:00:00.0+00'::timestamptz, '30 day', 'volume4', 'account2', 'sp2', 'c1',  2000000,   4, 1000004, 500000, 500000, 'cg2', ARRAY['AG4']);

-- To count the metrics generated, do something like:
--   SELECT count(*) from volumemetrics1 where vid = idof('volume4');
--
-- Sample query to return hourly average VALUES for R&W for a given volume in a given time range:
--
-- WITH q AS (
--     SELECT
--         time_bucket ('60 minutes', M.timestamp) AS TS,
--         M.VolumeMetadataNum,
--         count(*) AS cnt,
--         avg(BytesRead)::int AS AvgBytesRead,
--         avg(BytesWritten)::int AS AvgBytesWritten,
--         avg(NumberReads)::int AS AvgNumReads,
--         avg(NumberWrites)::int AS AvgNumWrites,
--         sum(NumberReads) + sum(NumberWrites) AS NumIO,
--         avg(LatencyMean)::int AS AvgLatency,
--         max(LatencyMax)::int AS MaxLatency,
--         avg(ViolationLatencyMean)::int AS VLA,
--         avg(ViolationLatencyMax)::int AS VLM,
--         avg(ViolationWorkloadRate)::int AS VWR,
--         avg(ViolationWorkloadMixRead)::int AS VWMR,
--         avg(ViolationWorkloadMixWrite)::int AS VWMW,
--         avg(ViolationWorkloadAvgSizeMin)::bigint AS VWSN,
--         avg(ViolationWorkloadAvgSizeMax)::bigint AS VWSX
--     FROM VolumeMetrics M
--     JOIN VolumeMetadata VM ON M.VolumeMetadataNum = VM.VolumeMetadataNum
--     WHERE
--         M.timestamp > '2018-01-01 00:00:00.0+00'::timestamptz
--         AND VM.VolumeNum = (SELECT vo.VolumeNum FROM VolumeObjects vo WHERE VolumeID = idof ('volume1'))
--         GROUP BY
--             ts,
--             M.VolumeMetadataNum
--         ORDER BY
--             ts ASC
-- )
-- SELECT
--     q.*,
--     VolumeID,
--     AccountID,
--     ServicePlanID,
--     to_char(ts, 'J') AS JDay
-- FROM q
-- JOIN VolumeMetadata1 VM1 ON q.VolumeMetadataNum = VM1.VolumeMetadataNum
-- ORDER BY ts ASC, VolumeNum asc
-- ;

-- Note that the idof() function is part of this fake support logic and would not be used in practice.
-- Instead the desired UUID would be specified instead.

-- Experiment with AG by inserting a small number of metrics:
-- SELECT generate_vm1('2017-01-01 00:00:00.0+00'::timestamptz, '20 min', 'volume1', 'account1', 'sp1', 'c1', 10000000, 0, 1000000, 500000, 500000,'cg1', ARRAY['AG1']);
-- SELECT generate_vm1('2017-01-01 00:20:00.0+00'::timestamptz, '40 min', 'volume1', 'account1', 'sp1', 'c1', 10000000, 0, 1000000, 500000, 500000, 'cg1', ARRAY['AG1', 'AG2']);
-- SELECT generate_vm1('2017-01-01 00:00:00.0+00'::timestamptz, '60 min', 'volume2', 'account1', 'sp1', 'c1', 10000000, 0, 1000000, 500000, 500000, 'cg1', ARRAY['AG2']);
--
-- View the metadata:
-- SELECT * FROM volumemetadata JOIN agmember USING(volumeMetadataNum) ORDER BY timestamp, volumenum;
--
-- There should be 4 records returned.  Note that at 20 minutes past the hour AG2 has 2 volumes; prior to that it had just one.
--
-- Sample roll up query on AG below (based on above timestamp data and a 10 minute bucket); it should produce 14 rows.
-- Vary the query with larger and smaller bucket sizes and observe (via the CNT column) when AG2's sample size increases
-- (sometimes @20min is at the bucket boundary and sometimes not).
--
-- WITH q AS (
--     SELECT
--         time_bucket ('10 minutes', M.timestamp) AS TS,
--         M.VolumeMetadataNum,
--         count(*) AS cnt,
--         avg(BytesRead)::int AS AvgBytesRead,
--         avg(BytesWritten)::int AS AvgBytesWritten,
--         avg(NumberReads)::int AS AvgNumReads,
--         avg(NumberWrites)::int AS AvgNumWrites,
--         sum(NumberReads) + sum(NumberWrites) AS NumIO,
--         avg(LatencyMean)::int AS AvgLatency,
--         max(LatencyMax)::int AS MaxLatency,
--         avg(ViolationLatencyMean)::int AS VLA,
--         avg(ViolationLatencyMax)::int AS VLM,
--         avg(ViolationWorkloadRate)::int AS VWR,
--         avg(ViolationWorkloadMixRead)::int AS VWMR,
--         avg(ViolationWorkloadMixWrite)::int AS VWMW,
--         avg(ViolationWorkloadAvgSizeMin)::bigint AS VWSN,
--         avg(ViolationWorkloadAvgSizeMax)::bigint AS VWSX
--     FROM VolumeMetrics M
--     JOIN VolumeMetadata VM Using(VolumeMetadataNum)
--     WHERE
--         M.timestamp > '2017-01-01 00:00:00.0+00'::timestamptz
--         GROUP BY
--             ts,
--             M.VolumeMetadataNum
--         ORDER BY
--             ts ASC, M.VolumeMetadataNum
-- )
-- SELECT
--     TS,
--     avg(AvgBytesRead)::int AS AvgBytesRead,
--     avg(AvgBytesWritten)::int AS AvgBytesWritten,
--     avg(AvgNumReads)::int AS AvgNumReads,
--     avg(AvgNumWrites)::int AS AvgNumWrites,
--     sum(NumIO) AS NumIO,
--     avg(AvgLatency)::int AS AvgLatency,
--     max(MaxLatency)::int AS MaxLatency,
--     avg(VLA)::int AS VLA,
--     avg(VLM)::int AS VLM,
--     avg(VWR)::int AS VWR,
--     avg(VWMR)::int AS VWMR,
--     avg(VWMW)::int AS VWMW,
--     avg(VWSN)::bigint AS VWSN,
--     avg(VWSX)::bigint AS VWSX,
--     sum(cnt) AS CNT,
--     ApplicationGroupNum AS AGN,
--     ApplicationGroupID
-- FROM q
-- JOIN AGMember USING(VolumeMetadataNum)
-- JOIN ApplicationGroupObjects USING(ApplicationGroupNum)
-- GROUP BY TS, ApplicationGroupNum, ApplicationGroupID
-- ORDER BY TS asc, ApplicationGroupNum
-- ;

-- Experiment with pool metrics
--
-- SELECT poolmetricsinsert1(current_timestamp - interval '30 days', idof('pool1'), idof('dom1'), 1000000, 1000000, 1000000);
-- SELECT poolmetricsinsert1(current_timestamp - interval '29 days', idof('pool1'), idof('dom1'), 1000000, 1000000, 50000);
-- SELECT poolmetricsinsert1(current_timestamp - interval '29 days', idof('pool1'), idof('dom1'), 1000000, 50000, 50000);
-- SELECT poolmetricsinsert1(current_timestamp - interval '20 days', idof('pool1'), idof('dom1'), 1000000, 50000, 0);
-- SELECT poolmetricsinsert1(current_timestamp - interval '20 days', idof('pool1'), idof('dom1'), 1000000, 0, 0);
-- SELECT poolmetricsinsert1(current_timestamp - interval '10 days', idof('pool1'), idof('dom1'), 2000000, 1000000, 1000000);
-- SELECT poolmetricsinsert1(current_timestamp - interval '9 days', idof('pool1'), idof('dom1'), 2000000, 1000000, 50000);
-- SELECT poolmetricsinsert1(current_timestamp - interval '9 days', idof('pool1'), idof('dom1'), 2000000, 50000, 50000);

-- Experiment with storage metrics
--
-- SELECT generate_sm1('2018-01-01 00:00:00.0+00'::timestamptz, '90 day', 'storage1', 'Amazon gp2', 10000000, 1000000);
-- SELECT generate_sm1('2018-01-01 00:00:00.0+00'::timestamptz, '90 day', 'storage2', 'Amazon gp2', 20000000, 2000000);

-- Experiment with spa metrics
--
-- SELECT spametricsinsert1(current_timestamp - interval '30 days', idof('spa1'), idof('dom1'), idof('cl1'), idof('sp1'), idof('ac1'), 1000000, 1000000);
-- SELECT spametricsinsert1(current_timestamp - interval '29 days', idof('spa1'), idof('dom1'), idof('cl1'), idof('sp1'), idof('ac1'), 1000000, 50000);
-- SELECT spametricsinsert1(current_timestamp - interval '29 days', idof('spa1'), idof('dom1'), idof('cl1'), idof('sp1'), idof('ac1'), 1000000, 50000);
-- SELECT spametricsinsert1(current_timestamp - interval '20 days', idof('spa1'), idof('dom1'), idof('cl1'), idof('sp1'), idof('ac1'), 1000000, 0);
-- SELECT spametricsinsert1(current_timestamp - interval '20 days', idof('spa1'), idof('dom1'), idof('cl1'), idof('sp1'), idof('ac1'), 1000000, 0);
-- SELECT spametricsinsert1(current_timestamp - interval '10 days', idof('spa1'), idof('dom1'), idof('cl1'), idof('sp1'), idof('ac1'), 2000000, 1000000);
-- SELECT spametricsinsert1(current_timestamp - interval '9 days', idof('spa1'), idof('dom1'), idof('cl1'), idof('sp1'), idof('ac1'), 2000000, 50000);
-- SELECT spametricsinsert1(current_timestamp - interval '9 days', idof('spa1'), idof('dom1'), idof('cl1'), idof('sp1'), idof('ac1'), 2000000, 50000);

-- illustrate a timestamp loop
CREATE OR REPLACE FUNCTION f1 (tStart timestamptz, dur varchar)
    RETURNS void
AS $$
DECLARE
    ts timestamptz;
BEGIN
    << tsloop >> FOR ts IN
    SELECT
        generate_series(tStart, tStart + CAST(dur AS interval), '5 min')
        LOOP
            raise notice '%', ts;
        END LOOP
            tsloop;
            END;
$$
LANGUAGE plpgsql;

-- random integer in range generator
CREATE OR REPLACE FUNCTION randbetween (low int, high int)
    RETURNS int
AS $$
BEGIN
    RETURN floor(random() * (high - low + 1) + low);
END;
$$
LANGUAGE plpgsql;

-- table with name to uuid mappings as names are easy for debugging
CREATE TABLE IF NOT EXISTS nu(
    n varchar PRIMARY KEY,
    nid uuid
);

-- idof returns a consistent uuid for the given input
CREATE OR REPLACE FUNCTION idof (pName varchar)
    RETURNS uuid
AS $$
DECLARE
    id uuid;
BEGIN
    SELECT
        nid INTO id
    FROM
        nu
    WHERE
        n = pName;
    IF found THEN
        RETURN id;
    END IF;
    INSERT INTO nu (n, nid)
    VALUES (pName, uuid_generate_v4 ())
    ON CONFLICT DO NOTHING;
    RETURN idof(pName);
END;
$$
LANGUAGE plpgsql;

-- vmi1 inserts a single fake volume metric record with random values
CREATE OR REPLACE FUNCTION vmi1(ts timestamptz, vname varchar, sampleDuration varchar) returns void as $$
DECLARE
    BytesRead bigint = randbetween(0, 1000) * 1024::bigint;
    BytesWritten integer = randbetween(0, 1000) * 1024::bigint;
    NumberReads integer = randbetween(0, 100);
    NumberWrites integer = randbetween(0, 100);
    LatencyMean integer = randbetween(20, 24);
    LatencyMax integer = LatencyMean + randbetween(0, 8);
    ViolationLatencyMean integer = case when randbetween(0, 100) <= 8 then 1 else 0 end;
    ViolationLatencyMax integer = case when randbetween(0, 100) <= 5 then randbetween(1, 10) else 0 end;
    ViolationWorkloadRate bigint = case when randbetween(0, 100) <= 3 then 1 else 0 end;
    ViolationWorkloadMixRead bigint = case when randbetween(0, 100) <= 5 then randbetween(100,500) else 0 end;
    ViolationWorkloadMixWrite bigint = case when randbetween(0, 100) <= 5 then randbetween(100,500) else 0 end;
    ViolationWorkloadAvgSizeMin bigint = case when randbetween(0, 100) <= 2 then randbetween(10,500) else 0 end;
    ViolationWorkloadAvgSizeMax bigint = case when randbetween(0, 100) <= 4 then randbetween(10000,50000) else 0 end;
    NumCacheReadUserHits integer = randbetween(0, 100);
    NumCacheReadUserTotal integer = randbetween(100, 150);
    NumCacheReadMetaHits integer = randbetween(0, 100);
    NumCacheReadMetaTotal integer = randbetween(100, 150);
BEGIN
    PERFORM VolumeMetricsInsert1(ts, idof(vname),
        BytesRead, BytesWritten,
        NumberReads, NumberWrites,
        LatencyMean, LatencyMax,
        EXTRACT(EPOCH FROM CAST(sampleDuration AS interval))::smallint,
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
);
END;
$$
LANGUAGE plpgsql;

-- generate_vm1 inserts fake volume metrics for a given volume and time range
-- It inserts a VolumeMetadata record to anchor the series of metrics records
CREATE OR REPLACE FUNCTION generate_vm1 (tStart timestamptz, dur varchar, vName varchar, aName varchar,
                                         spName varchar, clusterName varchar,
                                         pTotalBytes bigint, pCostBytes bigint, pBytesAllocated bigint, 
                                         pRequestedCacheSizeBytes bigint, pAllocatedCacheSizeBytes bigint,
                                         cgName varchar, agNames varchar[])
    RETURNS void
AS $$
DECLARE
    ts timestamptz;
    name varchar;
    agids uuid[];
    i int := 1;
BEGIN
    FOREACH name IN ARRAY agNames
    LOOP
       agids[i] := idof(name);
       i := i + 1;
    END LOOP;
    PERFORM VolumeMetadataInsert1(tStart, idof(vName), idof(aName), idof(spName), idof(clusterName),
                                  idof(cgName), agids, pTotalBytes, pCostBytes, pBytesAllocated, 
                                  pRequestedCacheSizeBytes, pAllocatedCacheSizeBytes);
    FOR ts IN
    SELECT
        generate_series(tStart, tStart + CAST(dur AS interval), '5 min')
        LOOP
            PERFORM vmi1 (ts, vname, '5 min');
        END LOOP;
END;
$$
LANGUAGE plpgsql;

-- smi1 inserts a single fake storage metric record with random values
CREATE OR REPLACE FUNCTION smi1(ts timestamptz, sname varchar, sampleDuration varchar) returns void as $$
DECLARE
    BytesRead bigint = randbetween(0, 1000) * 1024::bigint;
    BytesWritten integer = randbetween(0, 1000) * 1024::bigint;
    NumberReads integer = randbetween(0, 100);
    NumberWrites integer = randbetween(0, 100);
    LatencyMean integer = randbetween(20, 24);
    LatencyMax integer = LatencyMean + randbetween(0, 8);
    ViolationLatencyMean integer = case when randbetween(0, 100) <= 8 then 1 else 0 end;
    ViolationLatencyMax integer = case when randbetween(0, 100) <= 5 then randbetween(1, 10) else 0 end;
    ViolationWorkloadRate bigint = case when randbetween(0, 100) <= 1 then 1 else 0 end;
BEGIN
    PERFORM StorageMetricsInsert1(ts, idof(sname),
        BytesRead, BytesWritten,
        NumberReads, NumberWrites,
        LatencyMean, LatencyMax,
        EXTRACT(EPOCH FROM CAST(sampleDuration AS interval))::smallint,
        ViolationLatencyMean,
        ViolationLatencyMax,
        ViolationWorkloadRate
    );
END;
$$
LANGUAGE plpgsql;

-- generate_sm1 inserts fake storage metrics for a given storage object and time range
-- It inserts a StorageMetadata record to anchor the series of metrics records
CREATE OR REPLACE FUNCTION generate_sm1 (tStart timestamptz, dur varchar, sName varchar, stName varchar, pTotalBytes bigint, pAvailableBytes bigint)
    RETURNS void
AS $$
DECLARE
    ts timestamptz;
BEGIN
    INSERT INTO StorageMetadata(Timestamp, StorageNum, StorageTypeNum, TotalBytes, AvailableBytes)
    VALUES (tStart, StorageNumber(idof(sName)), StorageTypeNumber(stName), pTotalBytes, pAvailableBytes);
    FOR ts IN
    SELECT
        generate_series(tStart, tStart + CAST(dur AS interval), '5 min')
        LOOP
            PERFORM smi1 (ts, sName, '5 min');
        END LOOP;
END;
$$
LANGUAGE plpgsql;

-- volume meta table experiment1
-- CREATE TABLE IF NOT EXISTS vmeta(
--     Timestamp timestamptz NOT NULL,
--     VNum integer REFERENCES VObjects MATCH FULL ON DELETE CASCADE,
--     Attr1 varchar
-- );
-- CREATE UNIQUE INDEX IF NOT EXISTS vmetaI1 ON vmeta (Timestamp DESC, VNum);

-- insert into vmeta values('2018-01-01 00:00:00.0+00'::timestamptz, VNumber(idof('volume1')), 'Value 1');
-- insert into vmeta values('2018-02-01 00:00:00.0+00'::timestamptz, VNumber(idof('volume1')), 'Value 2');
-- insert into vmeta values('2018-03-01 00:00:00.0+00'::timestamptz, VNumber(idof('volume1')), 'Value 3');

-- left outer join over parallel queries. will show nulls for missing metadata
--            ts           | vnum | cnt  |  attr
-- ------------------------+------+------+---------
--  2017-12-27 00:00:00+00 |    1 | 7200 | Value 1
--  2018-01-26 00:00:00+00 |    1 | 8640 | Value 2
--  2018-02-25 00:00:00+00 |    1 | 8640 | Value 3
--  2018-03-27 00:00:00+00 |    1 | 1441 |
-- (4 rows)
-- with q1 as (
--         select time_bucket('30 days', timestamp) as TS, vnum, count(*) as cnt
--         from volumemetrics
--         where vnum = 1
--         group by ts, vnum
--         order by ts asc
--     ),
--     q2 as (
--         select time_bucket('30 days', timestamp) as TS, vnum, last(attr1, timestamp) as attr
--         from vmeta
--         where vnum = 1
--         group by ts, vnum
--         order by ts asc
--     )
-- select q1.*, q2.attr
-- from q1
-- left join q2 on q1.TS = q2.TS;

-- this uses parallel queries to compute the data and then a lateral join to combine
--            ts           | vnum | cnt  |  attr
-- ------------------------+------+------+---------
--  2017-12-27 00:00:00+00 |    1 | 7200 | Value 1
--  2018-01-26 00:00:00+00 |    1 | 8640 | Value 2
--  2018-02-25 00:00:00+00 |    1 | 8640 | Value 3
--  2018-03-27 00:00:00+00 |    1 | 1441 | Value 3
-- (4 rows)
-- with q1 as (
--         select time_bucket('30 days', timestamp) as TS, vnum, count(*) as cnt
--         from volumemetrics
--         where vnum = 1
--         group by ts, vnum
--         order by ts asc
--     ),
--     q2 as (
--         select time_bucket('30 days', timestamp) as TS, vnum, last(attr1, timestamp) as attr
--         from vmeta
--         where vnum = 1
--         group by ts, vnum
--         order by ts asc
--     )
-- select q1.*, X.attr
-- from q1
-- inner join lateral(
--     select last(attr, ts) as attr from q2 where q2.vnum = q1.vnum and q2.ts <= q1.ts
-- ) as X on true
-- ;

-- volume meta experiment2 with multiple fks
-- CREATE TABLE IF NOT EXISTS vm2(
--     Timestamp timestamptz NOT NULL,
--     VNum integer REFERENCES VObjects MATCH FULL ON DELETE CASCADE,
--     ACNum integer DEFAULT 0 REFERENCES ACObjects ON DELETE SET DEFAULT,
--     SPNum integer DEFAULT 0 REFERENCES SPObjects ON DELETE SET DEFAULT, -- can vary
--     TotalBytes bigint  -- can vary
-- );
-- CREATE UNIQUE INDEX IF NOT EXISTS vm2I1 ON vm2 (Timestamp DESC, VNum);

-- insert into vm2 values('2018-01-01 00:00:00.0+00'::timestamptz, VNumber(idof('volume1')), ACNumber(idof('account1')), SPNumber(idof('sp1')), 10000);
-- insert into vm2 values('2018-03-01 00:00:00.0+00'::timestamptz, VNumber(idof('volume1')), ACNumber(idof('account1')), SPNumber(idof('sp2')), 10000);

-- insert into vm2 values('2018-01-01 00:00:00.0+00'::timestamptz, VNumber(idof('volume2')), ACNumber(idof('account1')), SPNumber(idof('sp1')), 200000);
-- insert into vm2 values('2018-02-01 00:00:00.0+00'::timestamptz, VNumber(idof('volume2')), ACNumber(idof('account1')), SPNumber(idof('sp1')), 220000);
-- insert into vm2 values('2018-03-01 00:00:00.0+00'::timestamptz, VNumber(idof('volume2')), ACNumber(idof('account1')), SPNumber(idof('sp1')), 230000);
-- insert into vm2 values('2018-04-01 00:00:00.0+00'::timestamptz, VNumber(idof('volume2')), ACNumber(idof('account1')), SPNumber(idof('sp2')), 240000);

-- insert into vm2 values('2018-01-01 00:00:00.0+00'::timestamptz, VNumber(idof('volume3')), ACNumber(idof('account2')), SPNumber(idof('sp1')), 300000);
-- insert into vm2 values('2018-02-01 00:00:00.0+00'::timestamptz, VNumber(idof('volume3')), ACNumber(idof('account2')), SPNumber(idof('sp2')), 300000);

-- insert into vm2 values('2018-01-01 00:00:00.0+00'::timestamptz, VNumber(idof('volume4')), ACNumber(idof('account2')), SPNumber(idof('sp2')), 300000);
-- insert into vm2 values('2018-03-01 00:00:00.0+00'::timestamptz, VNumber(idof('volume4')), ACNumber(idof('account2')), SPNumber(idof('sp1')), 300000);

-- query filtering on metadata (account = account1)
--            ts           | vnum | cnt  | totalbytes | acnum | spnum
-- ------------------------+------+------+------------+-------+-------
--  2017-12-27 00:00:00+00 |    1 | 7200 |      10000 |     1 |     1
--  2017-12-27 00:00:00+00 |    2 | 7200 |     200000 |     1 |     1
--  2018-01-26 00:00:00+00 |    1 | 8640 |      10000 |     1 |     1
--  2018-01-26 00:00:00+00 |    2 | 8640 |     220000 |     1 |     1
--  2018-02-25 00:00:00+00 |    1 | 8640 |      10000 |     1 |     2
--  2018-02-25 00:00:00+00 |    2 | 8640 |     220000 |     1 |     1
--  2018-03-27 00:00:00+00 |    1 | 1441 |      10000 |     1 |     2
--  2018-03-27 00:00:00+00 |    2 | 1441 |     240000 |     1 |     2
-- (8 rows)
--
-- with q1 as (
--         select time_bucket('30 days', timestamp) as TS, vnum, count(*) as cnt
--         from volumemetrics
--         group by ts, vnum
--         order by ts asc, vnum asc
--     ),
--     q2 as (
--         select time_bucket('30 days', timestamp) as TS, vnum, acnum,
--                 last(spnum, timestamp) as spnum, last(totalBytes, timestamp) as TotalBytes
--         from vm2
--         where acnum = ACNumber(idof('account1'))
--         group by ts, vnum, acnum
--         order by ts asc, vnum asc
--     )
-- select q1.*, X.TotalBytes, X.acnum, X.spnum
-- from q1
-- join lateral(
--     select last(TotalBytes, ts) as TotalBytes, acnum, last(spnum, ts) as spnum from q2 where q2.vnum = q1.vnum and q2.ts <= q1.ts
--     group by Totalbytes, acnum limit 1
-- ) as X on true
-- ;

-- query filtering on metadata (spnum = sp2)
--            ts           | vnum | cnt  | totalbytes | acnum | spnum
-- ------------------------+------+------+------------+-------+-------
--  2017-12-27 00:00:00+00 |    4 | 7200 |     300000 |     2 |     2
--  2018-01-26 00:00:00+00 |    3 | 8640 |     300000 |     2 |     2
--  2018-01-26 00:00:00+00 |    4 | 8640 |     300000 |     2 |     2
--  2018-02-25 00:00:00+00 |    1 | 8640 |      10000 |     1 |     2
--  2018-02-25 00:00:00+00 |    3 | 8640 |     300000 |     2 |     2
--  2018-02-25 00:00:00+00 |    4 | 8640 |     300000 |     2 |     2
--  2018-03-27 00:00:00+00 |    1 | 1441 |      10000 |     1 |     2
--  2018-03-27 00:00:00+00 |    2 | 1441 |     240000 |     1 |     2
--  2018-03-27 00:00:00+00 |    3 | 1441 |     300000 |     2 |     2
--  2018-03-27 00:00:00+00 |    4 | 1441 |     300000 |     2 |     2
-- (10 rows)
--
-- with q1 as (
--         select time_bucket('30 days', timestamp) as TS, vnum, count(*) as cnt
--         from volumemetrics
--         group by ts, vnum
--         order by ts asc, vnum asc
--     ),
--     q2 as (
--         select time_bucket('30 days', timestamp) as TS, vnum, acnum,
--                 last(spnum, timestamp) as spnum, last(totalBytes, timestamp) as TotalBytes
--         from vm2
--         where spnum = SPNumber(idof('sp2'))
--         group by ts, vnum, acnum
--         order by ts asc, vnum asc
--     )
-- select q1.*, X.TotalBytes, X.acnum, X.spnum
-- from q1
-- join lateral(
--     select last(TotalBytes, ts) as TotalBytes, acnum, last(spnum, ts) as spnum from q2 where q2.vnum = q1.vnum and q2.ts <= q1.ts
--     group by Totalbytes, acnum limit 1
-- ) as X on true
-- ;

-- Can also add all the other columns and order the output further (implicitly ordered by ts but vnum was not ordered):
--
-- with q1 as (
--         select time_bucket('7 days', timestamp) as TS, vnum, count(*) as cnt,
--                 avg(BytesRead)::int AS AvgBytesRead,
--                 avg(BytesWritten)::int AS AvgBytesWritten,
--                 avg(NumberReads)::int AS AvgNumReads,
--                 avg(NumberWrites)::int AS AvgNumWrites,
--                 sum(NumberReads) + sum(NumberWrites) AS NumIO,
--                 avg(LatencyMean)::int AS AvgLatency,
--                 max(LatencyMax)::int AS MaxLatency
--         from volumemetrics
--         group by ts, vnum
--         order by ts asc, vnum asc
--     ),
--     q2 as (
--         select time_bucket('7 days', timestamp) as TS, vnum, acnum,
--                 last(spnum, timestamp) as spnum, last(totalBytes, timestamp) as TotalBytes
--         from vm2
--         where acnum = ACNumber(idof('account1'))
--         group by ts, vnum, acnum
--         order by ts asc, vnum asc
--     )
-- select q1.*, X.TotalBytes, X.acnum, X.spnum, to_char(ts, 'J') as JDay
-- from q1
-- join lateral(
--     select last(TotalBytes, ts) as TotalBytes, acnum, last(spnum, ts) as spnum from q2 where q2.vnum = q1.vnum and q2.ts <= q1.ts
--     group by Totalbytes, acnum limit 1
-- ) as X on true
-- order by ts asc, vnum asc
-- ;

