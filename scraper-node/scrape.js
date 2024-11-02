const axios = require('axios');
const {createClient, ClickHouseLogLevel} = require('@clickhouse/client');
const currentTimeStamp = Date.now();
const date = new Date(currentTimeStamp).toLocaleDateString('en-CA');

const clickClient = new createClient({
    url: 'http://localhost:8123',
    basicAuth: {
        username: 'default',
        password: ''
    },
    debug: false,
    format: 'json'
});
// To avoid blocking
const DelayTime = 25000; // 25s 
const delay = (ms)=> new Promise(resolve => setTimeout(resolve, ms));

const fetchData = (page_no) => axios.get(`https://www.daraz.com.np/computing/?ajax=true&page=${page_no}`)

async function fetchDataFromAPI(page_no){
    try {
        const response = await fetchData(page_no);
        const items = response.data.mods.listItems.map(el => {
            return {
                id: `${el.nid}-${date}`,
                name: el.name,
                itemId: el.itemId,
                priceShow: el.priceShow,
                discount: el.discount,
                ratingScore: parseFloat(el.ratingScore).toFixed(5),
                review: el.review,
                location: el.location,
                description: el.description.join(','),
                sellerName: el.sellerName,
                sellerId: el.sellerId,
                brandName: el.brandName,
                brandId: el.brandId,
                price: parseInt(el.price),
                inStock: el.inStock,
                itemSoldCntShow: parseInt(el.itemSoldCntShow),
                originalPrice: parseInt(el.originalPrice),
                itemUrl: el.itemUrl,
                page_no: page_no,
                scraped_date: date,
            }

        });

        await clickClient.insert({
            table: 'daraz',
            values: items,
            format: 'JSONEachRow'
        });
        // to avoid blocking

    } catch (error) {
        console.log(error)
    }
    await delay(DelayTime)
}

(async ()=>{
    try {     
        const res = await fetchData(1);
        const pageSize = parseInt(res.data.mainInfo.pageSize);
        const totalResults = parseInt(res.data.mainInfo.totalResults);
        const totalPages = totalResults / pageSize;
        if(isNaN(totalPages)) throw new Error('total Pages nan'+totalPages);
        console.log(` Total Page ${totalPages}`);
        for(let i = 1; i < totalPages; i++){
            await fetchDataFromAPI(i);
            console.log(`Page:${i} Done`);
        }
    } catch (error) {
        console.log(error);
        console.log('Error on Initial Request');
    }
})()