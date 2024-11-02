CREATE TABLE IF NOT EXISTS default.daraz (
    id String,
    name String,
    itemId String,
    priceShow String,
    discount String,
    ratingScore Float32,
    review String,
    location String,
    description String,
    sellerName String,
    sellerId String,
    brandName String,
    brandId String,
    price Int16,
    page_no Int16,
    scraped_date Date,
    itemUrl String,
    inStock Boolean,
    itemSoldCntShow Int16,
    originalPrice Int16,
) ENGINE = MergeTree()
ORDER BY id;

-- add column
ALTER TABLE daraz ADD COLUMN page_no Int16 [AFTER price];


-- select
select * from default.daraz

select count(*) from default.daraz

truncate table default.daraz

-- export to parquet
SELECT * FROM default.daraz INTO OUTFILE 'default.parquet' FORMAT Parquet;

-- basic database queries
show databases; -- list database
use {{database_name}} -- 
select currentDatabase(); --
show tables from {{database_name}}