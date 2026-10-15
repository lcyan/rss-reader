# 简述

## 特性

- 打包后镜像大小仅有约20MB，通过docker实现一键部署

- 支持自定义配置页面数据自动刷新

- 响应式布局，能够兼容不同的屏幕大小

- 良好的SEO，首次加载使用模版引擎快速展示页面内容

- 支持添加多个RSS订阅链接

- 简洁的页面布局，可以查看每个订阅链接最后更新时间

- 支持夜间模式

- config.json配置文件支持热更新

- ***在原作者基础上，进行二次开发，增加了识别关键词后，推送通知到飞书和Telegram（2024年6月2日）*** ***注意⚠：docker-compose.yml 中端口默认是9898***

- ***在二次开发基础上，进行三次开发，使配置的关键字支持正则表达式，优化了性能，并添加了黑名单功能，可以指定排除指定关键字***

  
2023年7月28日，进行了界面改版和升级

![](pc.png)

![](mobile.png)

# 配置文件

已提供 docker-compose 方式，可以一键完成安装启动运行 ***注意⚠：docker-compose.yml 中端口默认是9898***

部署前请先配置，配置都在 config.json 中修改，使用前请先去 config.json 中增加自己飞书机器人的webhook地址 或 Telegram 的 token 和 chat_id，注意⚠️ TG api 后面的地址不要改！ https://api.telegram.org/bot${token}/sendMessage ，也就是这个${token}保持原样别动

config.json 中的 refresh 单位为秒，表示多少秒请求一次所需的 rss 订阅源

TG机器人创建和权限赋予教程请看 https://www.telegramhcn.com/article/161.html

配置文件位于config.json，sources是RSS订阅链接，示例如下

```json
{
    "values": [
        "https://linux.do/latest.rss",
        "https://rss.nodeseek.com",
        "https://hostloc.com/forum.php?mod=rss&fid=45&auth=389ec3vtQanmEuRoghE%2FpZPWnYCPmvwWgSa7RsfjbQ%2BJpA%2F6y6eHAx%2FKqtmPOg",
        "https://v2ex.com/feed/tab/tech.xml",
        "https://www.dalao.net/feed.htm"
    ],
    "refresh": 5,
    "autoUpdatePush": 7,
    "nightStartTime": "06:30:00",
    "nightEndTime": "19:30:00",

    "keywords": ["cc","cloudcone","rn","racknerd","咸鱼","4837","jpp","hk2p"],
    "denyKeywords":[
        "^这是排除关键字"
    ],
    "notify" : {
        "feishu": {
            "api": ""
        },
        "telegram": {
            "api": "https://api.telegram.org/bot${token}/sendMessage",
            "chat_id":"",
            "token": ""
        }
    },
    "archives": "archives.txt"
}
```

名称 | 说明
-|-
values | rss订阅链接（必填）
refresh | rss订阅更新时间间隔，单位秒（必填）
autoUpdatePush | 自动刷新间隔，默认为0，不开启。效果为前端每autoUpdatePush分钟自动更新页面信息，单位分钟（非必填）
nightStartTime | 日间开始时间 ，如 06:30:00
nightEndTime | 日间结束时间，如 19:30:00
keywords | 关键词列表，支持正则表达式，匹配后推送通知（非必填）
denyKeywords | 排除关键词/黑名单，支持正则表达式，匹配后不推送（非必填）
notify | 通知配置，支持飞书 webhook 和 Telegram bot（非必填）
archives | 已推送文章记录归档文件路径，默认 archives.txt（必填）

# 使用方式

## Docker部署

环境要求：Git、Docker、Docker-Compose

### 1. 克隆仓库

```bash
git clone https://github.com/lcyan/rss-reader && cd rss-reader
```

### 2. 准备 data 目录

容器以非 root 用户（uid 1000）运行，需要提前创建 `data` 目录并设置权限：

```bash
mkdir -p data
cp config.json data/config.json
touch data/archives.txt
chown -R 1000:1000 data
```

> **说明**：`config.json` 为配置文件，`archives.txt` 用于记录已推送通知的文章链接（需可写）。

### 3. 修改配置

编辑 `data/config.json`，按需修改 RSS 订阅源、关键词、通知方式等，详见上方 [配置文件](#配置文件) 章节。

### 4. 启动服务

```bash
docker compose up -d
```

### 5. 验证

```bash
# 查看容器状态
docker compose ps

# 查看日志
docker compose logs -f
```

部署成功后，端口默认绑定 `127.0.0.1:9898`（仅本机可访问），需配合下方 nginx 反代通过域名访问。如需直接通过 IP 访问，可将 docker-compose.yml 中端口改为 `"9898:8080"`

# nginx反代

这里需要注意/ws，若不设置proxy_read_timeout参数，则默认1分钟断开。静态文件增加gzip可以大幅压缩网络传输数据

```conf
server {
    listen 443 ssl;
    server_name 域名;
    ssl_certificate  域名证书.cer;
    ssl_certificate_key 域名证书.key;
    gzip on;
    gzip_types text/plain text/css application/json application/javascript text/xml application/xml application/xml+rss text/javascript;
    location / {
        proxy_pass  http://localhost:9898;
    }
    location /ws {
        proxy_pass http://localhost:9898/ws;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "Upgrade";
        proxy_set_header Host $host;
        proxy_read_timeout 300s;
    }
}

server {
    listen 80;
    server_name 域名;
    rewrite ^(.*)$ https://$host$1 permanent;
}
```
